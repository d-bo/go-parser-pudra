package main

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
    //"gopkg.in/mgo.v2/bson"
    "golang.org/x/net/html"
    "github.com/go-redis/redis"
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
func ExtractBrand(glob_session *mgo.Session, url string, redis_cli *redis.Client) {

    var f func(*html.Node, *mgo.Session, *redis.Client)

    //var crumbs []string
    //collect := glob_session.DB("parser").C("pudra_navi")

    f = func(node *html.Node, session *mgo.Session, redis_cli *redis.Client) {
        if node.Type == html.ElementNode && node.Data == "a" {
            match := false
            contents := ""
            href := ""
            _ = href
            for _, a := range node.Attr {
                if a.Key == "class" {
                    if strings.Contains(a.Val, "b-menu-item-link") {
                        contents = renderNode(node)
                        contents = extractContext(contents)
                        contents = strings.Replace(contents, "\n", "", -1)
                        contents = strings.Replace(contents, "\r", "", -1)
                        contents = strings.Replace(contents, "\t", "", -1)
                        if len(contents) > 1 {
                            match = true
                        }
                    }
                }
                if a.Key == "href" {
                    href = a.Val
                }
            }
            if match {
                fmt.Println("FOUND b-menu-item-link", contents, len(contents), href)
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f(c, session, redis_cli)
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

    f(doc, glob_session, redis_cli)
}

func main() {

    // Redis
    client := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "", // no password set
        DB:       0,  // use default DB
    })
    pong, err := client.Ping().Result()
    fmt.Println(pong, err)

    // Mongo
    session, glob_err := mgo.Dial("mongodb://apidev:apidev@localhost:27017/parser")
    defer session.Close()

    pubsub := client.Subscribe("productPageLinkChannel")
    defer pubsub.Close()

    subscr, err := pubsub.ReceiveTimeout(time.Second)
    if err != nil {
        fmt.Println(err)
    }
    fmt.Println(subscr)

    if glob_err != nil {
        syslog.Critf("Error: %s", glob_err)
    }

    ExtractBrand(session, `https://pudra.ru/brands.html`, client)

    /*
    for {
        msg, err := pubsub.ReceiveMessage()
        if err != nil {
            fmt.Println("ERROR: ", err)
        }
        fmt.Println(msg.Channel, " MSG RCV: ", msg.Payload)
    }
    */
}