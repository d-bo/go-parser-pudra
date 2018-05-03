package main

import (
    "os"
    "io"
    "fmt"
    "time"
    //"sync"
    "bytes"
    //"strconv"
    "strings"
    "net/http"
    "io/ioutil"
    //"encoding/json"
    //"io/ioutil"
    //"gopkg.in/mgo.v2"
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

func main() {

    var f func(*html.Node)

    f = func(node *html.Node) {
        if node.Type == html.ElementNode && node.Data == "li" {
            contents := renderNode(node)
            if strings.Contains(contents, "Password") {
                contents = strings.Replace(contents, "<strong>", "", -1)
                contents = strings.Replace(contents, "</strong>", "", -1)
                contents = strings.Replace(contents, "<li>", "", -1)
                contents = strings.Replace(contents, "</li>", "", -1)
                contents = strings.Replace(contents, "Password:", "", -1)
                contents = strings.Replace(contents, "\n", "", -1)
                contents = strings.Replace(contents, "\r", "", -1)
                contents = strings.Replace(contents, "\t", "", -1)
                contents = strings.TrimLeft(contents, " ")
                contents = strings.TrimRight(contents, " ")
                contents = "vpnbook\n"+contents
                fmt.Println(contents)

                err := ioutil.WriteFile("/etc/openvpn/login.conf", []byte("contents"), 0775)
                if err != nil {
                    panic(err)
                }

                os.Exit(0)
            }
        }

        for c := node.FirstChild; c != nil; c = c.NextSibling {
            f(c)
        }
    }

    url := `https://www.vpnbook.com/freevpn`
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

    f(doc)
}