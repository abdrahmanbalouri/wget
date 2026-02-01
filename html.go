package main

import (
	"io"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

func extractLinks(r io.Reader) []string {
	var links []string
	doc, _ := html.Parse(r)

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "href" || a.Key == "src" {
					links = append(links, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return links
}

func convertHTMLLinks(path, domain string) {
	data, _ := os.ReadFile(path)
	html := string(data)

	html = strings.ReplaceAll(html, "https://"+domain, ".")
	html = strings.ReplaceAll(html, "http://"+domain, ".")

	os.WriteFile(path, []byte(html), 0o644)
}

func convertLinks(body []byte, host string) []byte {
	re := regexp.MustCompile(`(href|src)=["']https?://[^/]+(/[^"']*)["']`)
	return re.ReplaceAll(body, []byte(`$1="$2"`))
}
