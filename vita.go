package main

import (
    //"os"
    "io"
    "fmt"
    "time"
    "sync"
    "bytes"
    "strconv"
    "strings"
    "net/http"
    //"encoding/json"
    //"io/ioutil"
    "gopkg.in/mgo.v2"
    "gopkg.in/mgo.v2/bson"
    "golang.org/x/net/html"
    //"github.com/go-redis/redis"
    //"github.com/json-iterator/go"
    "github.com/blackjack/syslog"
    "github.com/parnurzeal/gorequest"
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

func Extract(glob_session *mgo.Session, url string, wg *sync.WaitGroup, ch chan int) {

    var f1 func(*html.Node, *mgo.Session)
    var f2 func(*html.Node, *mgo.Session)
    var f3 func(*html.Node, *mgo.Session)
    var f4 func(*html.Node, *mgo.Session)

    //var crumbs []string
    coll := glob_session.DB("parser").C(`VITA_products`)

    var Name string
    var Navi string
    var ListingPrice string
    var OldPrice string
    var Url string

    var crumbs []string

    f1 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "div" {
            for _, a := range node.Attr {
                if a.Val == "product__mobRight" {
                    fmt.Println("FOUND")
                    f2(node, session)

                    Navi = strings.Join(crumbs, ";")

                    dd, err := coll.Find(bson.M{"name": Name}).Count()
                    if err != nil {
                        syslog.Critf("pudra find price double: %s", err)
                    }
                    if dd < 1 {
                        // status = 0 -> pending
                        err := coll.Insert(bson.M{"navi": Navi, "name": Name, "oldprice": OldPrice, "listingprice": ListingPrice, "url": Url})
                        if err != nil {
                            syslog.Critf("pudra error insert: %s", err)
                        }
                    }

                    Name = ""
                    Navi = ""
                    OldPrice = ""
                    ListingPrice = ""

                    crumbs = []string{}
                }
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f1(c, session)
        }
    }

    f2 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "a" {
            for _, a := range node.Attr {
                if a.Key == "href" {
                    contents := renderNode(node)
                    contents = extractContext(contents)
                    contents = strings.Replace(contents, "\n", "", -1)
                    contents = strings.Replace(contents, "\r", "", -1)
                    contents = strings.Replace(contents, "\t", "", -1)
                    contents = strings.TrimLeft(contents, " ")
                    contents = strings.TrimRight(contents, " ")
                    if !strings.Contains(contents, "В корзину") {
                        //fmt.Println("NAME", contents)
                        fmt.Println("PRODUCT URL", a.Val)
                        Url = a.Val
                        request := gorequest.New()
                        resp, body, errs := request.Get("https://vitaexpress.ru"+a.Val).
                            Retry(3, 5 * time.Second, http.StatusBadRequest, http.StatusInternalServerError).
                            End()
                        _ = resp
                        if errs != nil {
                            syslog.Critf("vita request.Get(BrandUrl) error: %s", errs)
                        }

                        doc, err := html.Parse(strings.NewReader(string(body)))

                        if err != nil {
                            syslog.Critf("vita html.Parse error: %s", errs)
                        }

                        f3(doc, session)
                    }
                }
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f2(c, session)
        }
    }

    f3 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "h1" {
            for _, a := range node.Attr {
                if a.Val == "product-title" {
                    contents := renderNode(node)
                    contents = extractContext(contents)
                    contents = strings.Replace(contents, "\n", "", -1)
                    contents = strings.Replace(contents, "\r", "", -1)
                    contents = strings.Replace(contents, "\t", "", -1)
                    contents = strings.TrimLeft(contents, " ")
                    contents = strings.TrimRight(contents, " ")
                    Name = contents
                    fmt.Println("TITLE", contents)
                }
            }
        }

        if node.Type == html.ElementNode && node.Data == "div" {
            for _, a := range node.Attr {
                if a.Val == "product-price__cur" {
                    contents := renderNode(node)
                    contents = extractContext(contents)
                    contents = strings.Replace(contents, "\n", "", -1)
                    contents = strings.Replace(contents, "\r", "", -1)
                    contents = strings.Replace(contents, "\t", "", -1)
                    contents = strings.Replace(contents, `<span class="icon-rub"></span>`, "", -1)
                    contents = strings.TrimLeft(contents, " ")
                    contents = strings.TrimRight(contents, " ")
                    ListingPrice = unShufflePrice(contents)
                    fmt.Println("LISTING PRICE", ListingPrice)
                }
            }
        }

        if node.Type == html.ElementNode && node.Data == "div" {
            for _, a := range node.Attr {
                if a.Val == "product-price__old" {
                    contents := renderNode(node)
                    contents = extractContext(contents)
                    contents = strings.Replace(contents, "\n", "", -1)
                    contents = strings.Replace(contents, "\r", "", -1)
                    contents = strings.Replace(contents, "\t", "", -1)
                    contents = strings.Replace(contents, `<span class="icon-rub"></span>`, "", -1)
                    contents = strings.TrimLeft(contents, " ")
                    contents = strings.TrimRight(contents, " ")
                    OldPrice = unShufflePrice(contents)
                    fmt.Println("PRICE OLD", OldPrice)
                }
            }
        }

        if node.Type == html.ElementNode && node.Data == "ol" {
            for _, a := range node.Attr {
                if a.Val == "breadcrumb hidden-xs" {
                    f4(node, session)
                    fmt.Println(crumbs)
                }
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f3(c, session)
        }
    }

    f4 = func(node *html.Node, session *mgo.Session) {
        if node.Type == html.ElementNode && node.Data == "a" {
            match := false
            href := ""
            _ = href
            for _, a := range node.Attr {
                if a.Key == "href" {
                    href = a.Val
                    match = true
                }
            }
            if match {
                contents := renderNode(node)
                contents = extractContext(contents)
                contents = strings.Replace(contents, "\n", "", -1)
                contents = strings.Replace(contents, "\r", "", -1)
                contents = strings.Replace(contents, "\t", "", -1)
                //fmt.Println("CRUMB", contents)
                crumbs = append(crumbs, contents)
                match = false
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f4(c, session)
        }
    }

    fmt.Println("REQUEST URL:", url)
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

    f1(doc, glob_session)
}

func unShufflePrice(price string) string {

    var ns []rune

    for _, r := range price {
        switch r {
            case '1':
                ns = append(ns, '2')
            case '2':
                ns = append(ns, '4')
            case '3':
                ns = append(ns, '7')
            case '4':
                ns = append(ns, '3')
            case '5':
                ns = append(ns, '6')
            case '6':
                ns = append(ns, '5')
            case '7':
                ns = append(ns, '1')
            case '8':
                ns = append(ns, '9')
            case '9':
                ns = append(ns, '8')
            case '0':
                ns = append(ns, '0')
        }
    }

    return string(ns)
}

func main() {

    var wg sync.WaitGroup

    channel := make(chan int)

    // Mongo
    session, glob_err := mgo.Dial("mongodb://apidev:apidev@localhost:27017/parser")
    defer session.Close()

    if glob_err != nil {
        syslog.Critf("Error: %s", glob_err)
    }

    //wg.Add(1)
    i := 14
    for i < 50 {
        Extract(session, "https://vitaexpress.ru/ajax/filter.php?sort=SORT&direction=DESC&filters=2446%3Don%262458%3Don%262488%3Don%262507%3Don%262526%3Don%262551%3Don%262567%3Don%262579%3Don%262586%3Don&PAGEN_1="+strconv.Itoa(i), &wg, channel)
        i++
    }
    //wg.Wait()
    fmt.Println("Done")
}