package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (d *Downloader) mirrorWebsite(baseURL string, reject, exclude []string) {
	if d.client == nil {
		d.client = &http.Client{}
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		fmt.Fprintln(d.errWriter, "invalid url")
		return
	}

	domain := parsedBase.Host
	visited := make(map[string]bool)

	var crawl func(string)
	crawl = func(current string) {
		if visited[current] {
			return
		}
		visited[current] = true

		u, err := url.Parse(current)
		if err != nil || u.Host != domain {
			return
		}

		resp, err := d.client.Get(current)
		if err != nil || resp.StatusCode != http.StatusOK {
			return
		}
		defer resp.Body.Close()

		localPath := d.buildLocalPath(domain, u.Path)
		os.MkdirAll(filepath.Dir(localPath), 0o755)

		body, _ := io.ReadAll(resp.Body)
		os.WriteFile(localPath, body, 0o644)

		if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			return
		}

		links := extractLinks(bytes.NewReader(body))
		for _, link := range links {
			abs := resolveURL(current, link)
			if abs == "" || isRejected(abs, reject) || isExcluded(abs, exclude) {
				continue
			}
			crawl(abs)
		}

		if d.convertLinks {
			convertHTMLLinks(localPath, domain)
		}
	}

	start := baseURL
	if !strings.HasSuffix(start, "/") {
		start += "/"
	}
	crawl(start)
}

func isExcluded(link string, exclude []string) bool {
	u, _ := url.Parse(link)
	for _, p := range exclude {
		if strings.HasPrefix(u.Path, p) {
			return true
		}
	}
	return false
}

func isRejected(link string, reject []string) bool {
	for _, ext := range reject {
		if strings.HasSuffix(link, "."+ext) {
			return true
		}
	}
	return false
}

func (d *Downloader) buildLocalPath(domain, p string) string {
	if p == "" || p == "/" {
		p = "/index.html"
	}
	return filepath.Join(d.saveDir, domain, p)
}
