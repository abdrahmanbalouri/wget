package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (d *Downloader) mirrorWebsite(baseURL string, reject, exclude []string) {
	if d.client == nil {
		d.client = &http.Client{}
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		fmt.Fprintln(d.errWriter, "invalid url")
		return
	}

	domain := base.Host
	rootDir := filepath.Join(d.saveDir, domain)
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		fmt.Fprintln(d.errWriter, err)
		return
	}

	visited := make(map[string]bool)
	var mu sync.Mutex

	var crawl func(string)
	crawl = func(current string) {
		mu.Lock()
		if visited[current] {
			mu.Unlock()
			return
		}
		visited[current] = true
		mu.Unlock()

		currentURL, err := url.Parse(current)
		if err != nil || currentURL.Host != domain {
			return
		}
		if isExcluded(currentURL.Path, exclude) || isRejected(currentURL.Path, reject) {
			return
		}

		resp, err := d.client.Get(current)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(d.errWriter, "mirror skipped %s: status %s\n", current, resp.Status)
			return
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}

		localPath := d.buildLocalPath(domain, currentURL)
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		if err := os.WriteFile(localPath, body, 0o644); err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}

		contentType := resp.Header.Get("Content-Type")
		var links []string
		switch {
		case strings.Contains(contentType, "text/html"):
			links = extractLinksFromHTML(body)
		case strings.Contains(contentType, "text/css"):
			links = extractCSSLinks(body)
		default:
			return
		}

		localRewrites := make(map[string]string)
		for _, link := range links {
			abs := resolveURL(current, link)
			if abs == "" {
				continue
			}

			linkURL, err := url.Parse(abs)
			if err != nil || linkURL.Host != domain {
				continue
			}
			if isExcluded(linkURL.Path, exclude) || isRejected(linkURL.Path, reject) {
				continue
			}

			targetLocalPath := d.buildLocalPath(domain, linkURL)
			relative, err := filepath.Rel(filepath.Dir(localPath), targetLocalPath)
			if err == nil {
				localRewrites[link] = relative
				localRewrites[abs] = relative
				localRewrites[linkURL.Path] = relative
			}
			crawl(abs)
		}

		if d.convertLinks {
			if strings.Contains(contentType, "text/html") {
				convertHTMLLinks(localPath, localRewrites)
			}
			if strings.Contains(contentType, "text/css") {
				convertCSSLinks(localPath, localRewrites)
			}
		}
	}

	crawl(base.String())
	fmt.Fprintf(d.outWriter, "Downloaded [%s]\n", base.String())
	fmt.Fprintf(d.outWriter, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func isExcluded(targetPath string, exclude []string) bool {
	for _, p := range exclude {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(targetPath, p) {
			return true
		}
	}
	return false
}

func isRejected(link string, reject []string) bool {
	lower := strings.ToLower(link)
	for _, ext := range reject {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if ext == "" {
			continue
		}
		if strings.HasSuffix(lower, "."+ext) {
			return true
		}
	}
	return false
}

func (d *Downloader) buildLocalPath(domain string, u *url.URL) string {
	targetPath := u.Path
	if targetPath == "" || strings.HasSuffix(targetPath, "/") {
		targetPath = path.Join(targetPath, "index.html")
	}

	name := path.Base(targetPath)
	if !strings.Contains(name, ".") {
		targetPath = path.Join(targetPath, "index.html")
	}

	return filepath.Join(d.saveDir, domain, filepath.FromSlash(strings.TrimPrefix(targetPath, "/")))
}
