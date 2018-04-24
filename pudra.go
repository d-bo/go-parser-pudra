package main

import (
    //"os"
    "io"
    "fmt"
    "time"
    "sync"
    "bytes"
    //"strconv"
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

// Extract breadcrumbs
func ExtractBrand(glob_session *mgo.Session, url string, wg *sync.WaitGroup, ch chan int) {

    var f func(*html.Node, *mgo.Session)
    defer wg.Done()

    //var crumbs []string
    coll := glob_session.DB("parser").C(MakeTimePrefix(`pudra_navi`))

    f = func(node *html.Node, session *mgo.Session) {
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
                dd, err := coll.Find(bson.M{"brand": contents, "status": 0}).Count()
                if err != nil {
                    syslog.Critf("pudra find price double: %s", err)
                }
                if dd < 1 {
                    // status = 0 -> pending
                    err := coll.Insert(bson.M{"brand": contents, "url": href, "status": 0})
                    if err != nil {
                        syslog.Critf("pudra error insert: %s", err)
                    }
                }
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f(c, session)
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

    f(doc, glob_session)
}

func PullFromQueue(glob_session *mgo.Session, ch chan int) {
    coll := glob_session.DB("parser").C(MakeTimePrefix(`pudra_navi`))
    type Product struct {
        Articul, Name, Price, Country, Img, Brand, Navi, Url, Date string
    }
    for {
        duration := 500 * time.Millisecond
        time.Sleep(duration)
        num, err := coll.Find(bson.M{"status": 0}).Count()
        if err != nil {
            syslog.Critf("pudra find price double: %s", err)
        }
        if num > 0 {
            fmt.Println("PULL")
            err := coll.Find(bson.M{"brand": contents, "status": 0}).One()
        }
        fmt.Println("sync")
    }
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

    wg.Add(1)
    go ExtractBrand(session, `https://pudra.ru/brands.html`, &wg, channel)
    wg.Add(1)
    go PullFromQueue(session, channel)
    wg.Wait()
    fmt.Println("Done")
}