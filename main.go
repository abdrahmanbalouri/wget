package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	//"bytes"
)

const logFileName = "wget-log"

type Downloader struct {
	client       *http.Client
	rateLimit    int64
	startTime    time.Time
	outWriter    io.Writer
	errWriter    io.Writer
	saveDir      string
	saveName     string
	convertLinks bool
}

/* ===================== MAIN ===================== */

func main() {
	background := flag.Bool("B", false, "background")
	output := flag.String("O", "", "output name")
	prefix := flag.String("P", ".", "output directory")
	rateLimitStr := flag.String("rate-limit", "", "rate limit")
	inputFile := flag.String("i", "", "input file")
	mirror := flag.Bool("mirror", false, "mirror website")
	reject := flag.String("R", "", "reject extensions")
	exclude := flag.String("X", "", "exclude paths")
	convert := flag.Bool("convert-links", false, "convert links")

	flag.Parse()
	urls := flag.Args()

	d := &Downloader{
		client:       &http.Client{},
		outWriter:    os.Stdout,
		errWriter:    os.Stderr,
		saveDir:      *prefix,
		saveName:     *output,
		convertLinks: *convert,
	}

	if *background {
		f, err := os.Create(logFileName)
		if err != nil {
			log.Fatal(err)
		}
		d.outWriter = f
		d.errWriter = f
		fmt.Println(`Output will be written to "wget-log".`)
	}

	if *rateLimitStr != "" {
		d.rateLimit = parseRateLimit(*rateLimitStr)
	}

	d.startTime = time.Now()
	fmt.Fprintf(d.outWriter, "start at %s\n", d.startTime.Format("2006-01-02 15:04:05"))

	if *mirror {
		if len(urls) == 0 {
			log.Fatal("mirror requires URL")
		}
		d.mirrorWebsite(urls[0],
			strings.Split(*reject, ","),
			strings.Split(*exclude, ","),
		)
		return
	}

	if *inputFile != "" {
		urls = readLines(*inputFile)
	}

	if len(urls) == 1 {
		d.downloadSingle(urls[0])
	} else {
		d.downloadMultiple(urls)
	}
}

/* ===================== DOWNLOAD ===================== */

func (d *Downloader) downloadSingle(rawURL string) {
	resp, err := d.client.Head(rawURL)
	if err != nil || resp.StatusCode != 200 {
		log.Fatal("request failed")
	}
	size := resp.ContentLength
	resp.Body.Close()

	fmt.Fprintf(d.outWriter, "sending request, awaiting response... status 200 OK\n")
	fmt.Fprintf(d.outWriter, "content size: %d [%s]\n", size, humanSize(size))

	name := d.saveName
	if name == "" {
		name = path.Base(rawURL)
	}
	savePath := filepath.Join(d.saveDir, name)
	os.MkdirAll(filepath.Dir(savePath), 0o755)

	fmt.Fprintf(d.outWriter, "saving file to: %s\n", savePath)

	r, _ := d.client.Get(rawURL)
	reader := r.Body
	if d.rateLimit > 0 {
		reader = newRateLimitedReader(r.Body, d.rateLimit)
	}
	defer reader.Close()

	f, _ := os.Create(savePath)
	defer f.Close()

	p := newProgress(size, d.outWriter)
	io.Copy(io.MultiWriter(f, p), reader)
	p.finish()

	fmt.Fprintf(d.outWriter, "\nDownloaded [%s]\n", rawURL)
	fmt.Fprintf(d.outWriter, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func (d *Downloader) downloadMultiple(urls []string) {
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			tmp := *d
			tmp.saveName = ""
			tmp.downloadSingle(u)
		}(u)
	}
	wg.Wait()
}

/* ===================== MIRROR ===================== */

func (d *Downloader) mirrorWebsite(baseURL string, reject, exclude []string) {
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
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return
		}

		localPath := d.buildLocalPath(domain, u.Path)
		os.MkdirAll(filepath.Dir(localPath), 0o755)

		body, _ := io.ReadAll(resp.Body)
		os.WriteFile(localPath, body, 0o644)

		// parse only HTML
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			return
		}

		links := extractLinks(bytes.NewReader(body))

		for _, link := range links {
			abs := resolveURL(current, link)
			if abs == "" {
				continue
			}

			if isRejected(abs, reject) || isExcluded(abs, exclude) {
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

/*--------------------------help-------------------*/
func isExcluded(link string, exclude []string) bool {
	u, _ := url.Parse(link)
	for _, path := range exclude {
		if strings.HasPrefix(u.Path, path) {
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

func (d *Downloader) buildLocalPath(domain, path string) string {
	if path == "" || path == "/" {
		path = "/index.html"
	}
	return filepath.Join(d.saveDir, domain, path)
}

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

/* ===================== CONVERT LINKS ===================== */

func convertLinks(body []byte, host string) []byte {
	re := regexp.MustCompile(`(href|src)=["']https?://[^/]+(/[^"']*)["']`)
	return re.ReplaceAll(body, []byte(`$1="$2"`))
}

/* ===================== HELPERS ===================== */

func resolveURL(base, ref string) string {
	b, _ := url.Parse(base)
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return b.ResolveReference(r).String()
}

func readLines(file string) []string {
	f, _ := os.Open(file)
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, strings.TrimSpace(sc.Text()))
	}
	return out
}

func parseRateLimit(s string) int64 {
	s = strings.ToLower(s)
	m := int64(1)
	if strings.HasSuffix(s, "k") {
		m = 1024
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "m") {
		m = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n * m
}

func humanSize(b int64) string {
	return fmt.Sprintf("~%.2fMB", float64(b)/(1024*1024))
}

/* ===================== PROGRESS ===================== */

type progressWriter struct {
	total int64
	done  int64
	start time.Time
	w     io.Writer
}

func newProgress(t int64, w io.Writer) *progressWriter {
	return &progressWriter{total: t, start: time.Now(), w: w}
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	atomic.AddInt64(&p.done, int64(n))
	perc := float64(p.done) * 100 / float64(p.total)
	fmt.Fprintf(p.w, "\r%.2f%%", perc)
	return n, nil
}

func (p *progressWriter) finish() {
	fmt.Fprintln(p.w)
}

/* ===================== RATE LIMIT ===================== */

type rateLimitedReadCloser struct {
	rc    io.ReadCloser
	limit int64
	read  int64
	start time.Time
}

func newRateLimitedReader(rc io.ReadCloser, l int64) io.ReadCloser {
	return &rateLimitedReadCloser{rc: rc, limit: l, start: time.Now()}
}

func (r *rateLimitedReadCloser) Read(p []byte) (int, error) {
	n, err := r.rc.Read(p)
	r.read += int64(n)
	allowed := int64(float64(r.limit) * time.Since(r.start).Seconds())
	if r.read > allowed {
		time.Sleep(50 * time.Millisecond)
	}
	return n, err
}

func (r *rateLimitedReadCloser) Close() error {
	return r.rc.Close()
}
