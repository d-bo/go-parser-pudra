package gapple

import (
    //"os"
    "io"
    "fmt"
    "time"
    "bytes"
    //"strconv"
    "strings"
    "net/http"
    //"encoding/json"
    //"io/ioutil"
    "gopkg.in/mgo.v2"
    "gopkg.in/mgo.v2/bson"
    "golang.org/x/net/html"
    "github.com/go-redis/redis"
    "github.com/json-iterator/go"
    "github.com/blackjack/syslog"
    "github.com/robertkrimen/otto"
    "github.com/parnurzeal/gorequest"
    "github.com/robertkrimen/otto/parser"
)

// A time prefix before collection name
func MakeTimePrefix(coll string) string {
    t := time.Now()
    ti := t.Format("02-01-2006")
    if coll == "" {
        return ti
    }
    fin := ti + "_" + coll
    return fin
}

// Render node
func renderNode(node *html.Node) string {
    var buf bytes.Buffer
    w := io.Writer(&buf)
    err := html.Render(w, node)
    if err != nil {
        syslog.Critf("Error: %s", err)
    }
    return buf.String()
}

// Get tag context
// TODO: prevent endless loop
func extractContext(s string) string {
    z := html.NewTokenizer(strings.NewReader(s))

    for {
        tt := z.Next()
        switch tt {
            case html.ErrorToken:
                syslog.Critf("auchan extractContext() error: %s", z.Err())
                syslog.Critf("String: %s", s)
                return ""
            case html.TextToken:
                text := string(z.Text())
                return text
        }
    }
}

// Extract url matching "/pokupki
func ExtractLinks(glob_session *mgo.Session, url string, redis_cli *redis.Client, keyword string) {

    var f func(*html.Node, *mgo.Session, *redis.Client)

    f = func(node *html.Node, session *mgo.Session, redis_cli *redis.Client) {
        if node.Type == html.ElementNode && node.Data == "a" {
            for _, a := range node.Attr {
                if a.Key == "href" {
                    if strings.Contains(a.Val, "pokupki/"+keyword) && strings.Contains(a.Val, "html") {
                        fmt.Println("#", "pokupki/"+keyword, a.Val)
                        err := redis_cli.Publish("productPageLinkChannel", a.Val).Err()
                        if err != nil {
                            panic(err)
                        }
                    }
                }
            }
        }

        // iterate inner nodes recursive
        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f(c, session, redis_cli)
        }
    }

    request := gorequest.New()
    resp, body, errs := request.Get(url).
        Retry(3, 5 * time.Second, http.StatusBadRequest, http.StatusInternalServerError).
        End()
    _ = resp
    if errs != nil {
        syslog.Critf("auchan request.Get(BrandUrl) error: %s", errs)
    }

    doc, err := html.Parse(strings.NewReader(string(body)))

    if err != nil {
        syslog.Critf("auchan html.Parse error: %s", errs)
    }

    f(doc, glob_session, redis_cli)
}

// Extract breadcrumbs
func ExtractNavi(glob_session *mgo.Session, url string, redis_cli *redis.Client) {

    var f func(*html.Node, *mgo.Session, *redis.Client)
    var f1 func(*html.Node, *mgo.Session)
    var f2 func(*html.Node, *mgo.Session)
    var f3 func(*html.Node, *mgo.Session)
    var f4 func(*html.Node, *mgo.Session, *redis.Client)

    var crumbs []string
    var crumbsTail string
    var contents string
    var finalCrumb string
    var tails []string
    var newone, double int

    collect := glob_session.DB("parser").C("auchan_navi")

    newone = 0
    double = 0
    var ProdName string

    f = func(node *html.Node, session *mgo.Session, redis_cli *redis.Client) {
        if node.Type == html.ElementNode && node.Data == "ul" {
            for _, a := range node.Attr {
                if a.Key == "class" {
                    if strings.Contains(a.Val, "breadcrumbs__list") {
                        f1(node, session)
                        f2(node, session)
                        if crumbsTail != "" {
                            crumbs = append(crumbs, crumbsTail)
                        }
                        finalCrumb = strings.Join(crumbs, ";")

                        for i := 0; i < len(tails); i++ {
                            // Insert Mongo
                            // check double product
                            ProdName = tails[i]
                            num, err := collect.Find(bson.M{"name": ProdName, "navi": finalCrumb}).Count()
                            if err != nil {
                                syslog.Critf("auchan error find by code: %s", err)
                            }
                            if num < 1 {
                                collect.Insert(bson.M{"navi": finalCrumb, "name": tails[i]})
                                newone++
                            } else {
                                fmt.Println("DOUBLE")
                                double++
                            }
                            fmt.Println("NAVI", finalCrumb)
                            fmt.Println("NAME", tails[i])
                        }
                        tails = []string{}
                        ProdName = ""
                    }
                }
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f(c, session, redis_cli)
        }
    }

    // Bc active links
    f1 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "a" {
            for _, a := range node.Attr {
                if a.Key == "href" {
                    if strings.Contains(a.Val, "pokupki") {
                        contents = renderNode(node)
                        contents = extractContext(contents)
                        if !strings.Contains(contents, "Страница") {
                            crumbs = append(crumbs, contents)
                        }
                    }
                }
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f1(c, session)
        }
    }

    // Breadcrumbs tail
    f2 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "strong" {
            contents = renderNode(node)
            contents = extractContext(contents)
            if !strings.Contains(contents, "Страница") {
                crumbsTail = contents
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f2(c, session)
        }
    }

    // Crumbs active links
    // JSON Extract name from this
    /*
        {
            "id": "145666",
            "type": "simple",
            "name": "Туалетная вода «Cool Water» Davidoff, 75 мл ",
            "brand": "not_set",
            "category": "Мужские ароматы",
            "list": "Catalog Page",
            "position": "5",
            "listPosition": "4"
        },
        {
            "id": "124546",
            "type": "simple",
            "name": "Туалетная вода «Gucci By Gucci Pour Homme» Gucci, 50 мл",
            "brand": "not_set",
            "category": "Мужские ароматы",
            "list": "Catalog Page",
            "position": "6",
            "listPosition": "5"
        },
    */

    f3 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "script" {
            contents = renderNode(node)
            contents = extractContext(contents)
            if strings.Contains(contents, "staticImpressions['product_list']") {
                tails = ExtractJSON(contents)
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f3(c, session)
        }
    }

    // Detect pagination
    // Push "next" link to redis queue
    f4 = func(node *html.Node, session *mgo.Session, redis_cli *redis.Client) {
        if node.Type == html.ElementNode && node.Data == "link" {
            match := false
            var value string
            for _, a := range node.Attr {
                if a.Key == "href" {
                    value = a.Val
                }
                if a.Val == "next" {
                    match = true
                }
            }

            if match {
                fmt.Println("PUSH/EXTRACT", value)
                ExtractNavi(session, value, redis_cli)
                match = false
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f4(c, session, redis_cli)
        }
    }

    fmt.Println("#ExtractNavi:", url)
    request := gorequest.New()
    resp, body, errs := request.Get(url).
        Retry(3, 5 * time.Second, http.StatusBadRequest, http.StatusInternalServerError).
        End()
    _ = resp
    if errs != nil {
        syslog.Critf("auchan request.Get(BrandUrl) error: %s", errs)
    }

    doc, err := html.Parse(strings.NewReader(string(body)))

    if err != nil {
        syslog.Critf("auchan html.Parse error: %s", errs)
    }

    f3(doc, glob_session)
    f4(doc, glob_session, redis_cli) // Pagination push
    f(doc, glob_session, redis_cli)
    fmt.Println("NEW", newone, "DOUBLE", double)
}

func ExtractJSONOtto(context string) string {
    vm := otto.New()
    vm.Run(context)
    if value, err := vm.Get("staticImpressions"); err == nil {
        fmt.Println("staticImpressions", value)
    }
    filename := ""
    program, err := parser.ParseFile(nil, filename, context, 0)
    if err != nil {
        fmt.Println(err)
    }
    for _, ll := range program.DeclarationList {
        fmt.Println("PROGRAM: ", ll)
    }

    return "0"
}

// Extract JSON
// Breadcrumb tails
func ExtractJSON(context string) []string {

    var i int
    var rec bool
    var strbuff []byte
    var dat interface{}
    var needle = "staticImpressions['product_list']"
    var Names []string

    num := strings.Index(context, needle)

    rec = false
    for i = (num + len(needle) + 1); i < len(context); i++ {
        // Start
        if string(context[i]) == "[" && rec == false {
            fmt.Println("Start [", i)
            rec = true
        }
        // Stop
        if string(context[i]) == "]" && rec == true {
            fmt.Println("Stop ]", i)
            break
        }
        if rec == true {
            strbuff = append(strbuff, context[i])
        }
    }

    var json = jsoniter.ConfigCompatibleWithStandardLibrary
    strbuffstr := []byte(string(strbuff)+"{}]")
    json.Unmarshal(strbuffstr, &dat)

    for k, v := range dat.([]interface{}) {
        switch t := v.(type) {
            case interface{}:
                for ck, cv := range t.(map[string]interface{}) {
                    if strings.Contains(ck, "name") {
                        Names = append(Names, cv.(string))
                    }
                }
            default:
                fmt.Println("wrong type")
        }
        //fmt.Printf("key[%s] value[%s]\n", k, v)
        _ = k
        _ = v
    }

    return Names
}

func main() {

    client := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "", // no password set
        DB:       0,  // use default DB
    })

    pong, err := client.Ping().Result()
    fmt.Println(pong, err)

    pubsub := client.Subscribe("productPageLinkChannel")
    defer pubsub.Close()

    subscr, err := pubsub.ReceiveTimeout(time.Second)
    if err != nil {
        fmt.Println(err)
    }
    fmt.Println(subscr)

    session, glob_err := mgo.Dial("mongodb://apidev:apidev@localhost:27017/parser")
    defer session.Close()

    if glob_err != nil {
        syslog.Critf("Error: %s", glob_err)
    }
    if DB == "" {
        DB = "parser"
    }

    gapple.ExtractLinks(session, "https://www.auchan.ru/pokupki/hoztovary.html", client, "hoztovary")
    gapple.ExtractLinks(session, "https://www.auchan.ru/pokupki/kosmetika/dlja-muzhchin/muzhskie-aromaty.html", client, "kosmetika")
    gapple.ExtractLinks(session, "https://www.auchan.ru/pokupki/deti.html", client, "deti")

    //gapple.ExtractNavi(session, "https://www.auchan.ru/pokupki/kosmetika/dlja-muzhchin.html", client)

    for {
        msg, err := pubsub.ReceiveMessage()
        if err != nil {
            fmt.Println("ERROR: ", err)
        }
        fmt.Println(msg.Channel, " MSG RCV: ", msg.Payload)
        gapple.ExtractNavi(session, msg.Payload, client)
    }
}