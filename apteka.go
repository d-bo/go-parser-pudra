package main

import (
    //"os"
    "io"
    "fmt"
    "time"
    //"sync"
    "bytes"
    //"strconv"
    "strings"
    "net/http"
    //"encoding/json"
    //"io/ioutil"
    "gopkg.in/mgo.v2"
    //"gopkg.in/mgo.v2/bson"
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

type MySession struct {
    *mgo.Session
}

func (session *MySession) ExtractProd(prod_ch chan string) {
    for {
        //time.Sleep(500 * time.Millisecond)
        select {
            case msg := <-prod_ch:
                fmt.Println("ExtractProd received:", "https://apteka.ru" + msg)
                /*
                request := gorequest.New()
                resp, body, errs := request.Get("https://apteka.ru" + msg).
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
                */
                //f2(doc, session)
            default:
                //fmt.Println("ExtractPage no msg rcvd")
        }
    }
}

// Page extract goroutine
func (session *MySession) ExtractPage(ch chan string, prod_ch chan string) {

    var f2 func(*html.Node, *MySession)

    f2 = func(node *html.Node, session *MySession) {
        if node.Type == html.ElementNode && node.Data == "a" {
            match := false
            href := ""
            for _, a := range node.Attr {
                if a.Key == "href" {
                    href = a.Val
                }
                if a.Key == "itemprop" && a.Val == "name" {
                    match = true
                }
            }
            if match {
                prod_ch <- href
                match = false
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f2(c, session)
        }
    }

    for {
        //time.Sleep(500 * time.Millisecond)
        select {
            case msg := <-ch:
                fmt.Println("ExtractPage received:", "https://apteka.ru" + msg)
                request := gorequest.New()
                resp, body, errs := request.Get("https://apteka.ru" + msg).
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
                f2(doc, session)
            default:
                //fmt.Println("ExtractPage no msg rcvd")
        }
    }
}

func (session *MySession) Extract(url string, ch chan string, prod_ch chan string) {

    var f2 func(*html.Node, *MySession)
    var f3 func(*html.Node, *MySession)
    var f4 func(*html.Node, *MySession)

    //var crumbs []string
    //coll := session.DB("parser").C(`VITA_products`)

    var Name string
    //var Navi string
    var ListingPrice string
    var OldPrice string
    //var Url string

    var crumbs []string

    f2 = func(node *html.Node, session *MySession) {
        if node.Type == html.ElementNode && node.Data == "a" {
            match := false
            href := ""
            for _, a := range node.Attr {
                if a.Key == "href" {
                    href = a.Val
                }
                if a.Key == "data-page" {
                    match = true
                }
            }
            if match {
                fmt.Println("found", href)
                ch <- href
                match = false
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f2(c, session)
        }
    }

    f3 = func(node *html.Node, session *MySession) {
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

    f4 = func(node *html.Node, session *MySession) {
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

    f2(doc, session)
}

func main() {

    //var wg sync.WaitGroup
    var pages [5]string

    pages[0] = "https://apteka.ru/category/derma_cosmetics/body/"
    pages[1] = "https://apteka.ru/category/derma_cosmetics/head/"
    pages[2] = "https://apteka.ru/category/derma_cosmetics/baby/"
    pages[3] = "https://apteka.ru/category/derma_cosmetics/face/"
    pages[4] = "https://apteka.ru/category/derma_cosmetics/sunscreen/"

    channel := make(chan string)
    prod_channel := make(chan string)

    // Mongo
    msession, glob_err := mgo.Dial("mongodb://apidev:apidev@localhost:27017/parser")
    defer msession.Close()

    session := &MySession{msession}

    if glob_err != nil {
        syslog.Critf("Error: %s", glob_err)
    }

    go session.ExtractPage(channel, prod_channel)
    go session.ExtractProd(prod_channel)

    i := 1
    for _, v := range pages {
        session.Extract(v, channel, prod_channel)
        i++
    }
    fmt.Println("Done")
}