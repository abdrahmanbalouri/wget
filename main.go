package main

import (
	"bufio"
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
)

const logFileName = "wget-log"

type Downloader struct {
	client    *http.Client
	rateLimit int64 // bytes/sec, 0 = unlimited
	startTime time.Time
	outWriter io.Writer
	errWriter io.Writer
	saveDir   string
	saveName  string
}

func main() {
	var (
		background   = flag.Bool("B", false, "Download in background")
		output       = flag.String("O", "", "Save file with this name")
		prefix       = flag.String("P", ".", "Save files to this directory")
		rateLimitStr = flag.String("rate-limit", "", "Limit download rate (e.g. 400k, 2M)")
		inputFile    = flag.String("i", "", "Read URLs from this file")
		mirror       = flag.Bool("mirror", false, "Mirror the website")
		reject       = flag.String("R", "", "Reject suffixes (comma separated: jpg,gif)")
		exclude      = flag.String("X", "", "Exclude directories (comma separated: /js,/css)")
	)

	flag.Parse()
	urls := flag.Args()
	d := &Downloader{
		client:    &http.Client{},
		outWriter: os.Stdout,
		errWriter: os.Stderr,
		saveDir:   *prefix,
		saveName:  *output,
	}

	// Background mode
	if *background {
		f, err := os.Create(logFileName)
		if err != nil {
			log.Fatal(err)
		}
		d.outWriter = f
		d.errWriter = f
		fmt.Printf("Output will be written to \"%s\".\n", logFileName)
	}

	// Rate limit
	if *rateLimitStr != "" {
		d.rateLimit = parseRateLimit(*rateLimitStr)
	}

	d.startTime = time.Now()
	fmt.Fprintf(d.outWriter, "start at %s\n", d.startTime.Format("2006-01-02 15:04:05"))

	// Mirror mode
	if *mirror {
		if len(urls) == 0 {
			fmt.Fprintln(d.errWriter, "Error: --mirror requires a URL")
			flag.Usage()
			os.Exit(1)
		}
		rejectList := strings.Split(*reject, ",")
		excludeList := strings.Split(*exclude, ",")
		d.mirrorWebsite(urls[0], rejectList, excludeList)
		return
	}

	// Input file mode
	if *inputFile != "" {
		urls = readLines(*inputFile)
	}

	if len(urls) == 0 {
		fmt.Fprintln(d.errWriter, "Error: No URL provided")
		flag.Usage()
		os.Exit(1)
	}

	if len(urls) > 1 {
		d.downloadMultiple(urls)
	} else {
		fmt.Println(urls)
		d.downloadSingle(urls[0])
	}
}

// ====================== Single Download ======================
func (d *Downloader) downloadSingle(rawURL string) {
	resp, err := d.client.Head(rawURL)
	fmt.Println(err)
	if err != nil || resp.StatusCode != http.StatusOK {
		status := "unknown"
		if resp != nil {
			status = resp.Status
		}
		fmt.Fprintf(d.errWriter, "sending request, awaiting response... status %s\n", status)
		os.Exit(1)
		
	}
	contentLength := resp.ContentLength
	resp.Body.Close()

	fmt.Fprintf(d.outWriter, "sending request, awaiting response... status 200 OK\n")
	fmt.Fprintf(d.outWriter, "content size: %d [%s]\n", contentLength, humanSize(contentLength))

	filename := d.saveName
	fmt.Println(filename,"++++++++")
	if filename == "" {
		filename = path.Base(rawURL)
		if filename == "" || filename == "/" || filename == "." {
			filename = "index.html"
		}
	}
	savePath := filepath.Join(d.saveDir, filename)
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		fmt.Fprintf(d.errWriter, "Error creating directory: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(d.outWriter, "saving file to: %s\n", savePath)

	getResp, err := d.client.Get(rawURL)
	if err != nil {
		fmt.Fprintf(d.errWriter, "Download error: %v\n", err)
		os.Exit(1)
	}

	reader := getResp.Body
	if d.rateLimit > 0 {
		reader = newRateLimitedReader(getResp.Body, d.rateLimit)
	}
	defer reader.Close()

	file, err := os.Create(savePath)
	if err != nil {
		fmt.Fprintf(d.errWriter, "Cannot create file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	progress := newProgress(contentLength, d.outWriter)
	_, err = io.Copy(io.MultiWriter(file, progress), reader)
	if err != nil {
		fmt.Fprintf(d.errWriter, "Download failed: %v\n", err)
		os.Exit(1)
	}
	progress.finish()

	fmt.Fprintf(d.outWriter, "\nDownloaded [%s]\n", rawURL)
	fmt.Fprintf(d.outWriter, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

// ====================== Multiple Downloads ======================
func (d *Downloader) downloadMultiple(urls []string) {
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			tempD := *d
			if len(urls) > 1 {
				tempD.saveName = ""
			}
			tempD.downloadSingle(url)
			fmt.Fprintf(d.outWriter, "finished %s\n", path.Base(url))
		}(u)
	}
	wg.Wait()

	fmt.Fprint(d.outWriter, "Download finished: [")
	for i, u := range urls {
		if i > 0 {
			fmt.Fprint(d.outWriter, " ")
		}
		fmt.Fprint(d.outWriter, u)
	}
	fmt.Fprintln(d.outWriter, "]")
}

// ====================== Mirroring ======================
func (d *Downloader) mirrorWebsite(baseURL string, reject, exclude []string) {
	baseParsed, err := url.Parse(baseURL)
	if err != nil {
		log.Fatal(err)
	}

	// ---------- reject extensions ----------
	rejectMap := make(map[string]bool)
	for _, s := range reject {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		rejectMap[s] = true
	}

	// ---------- exclude directories ----------
	excludeMap := make(map[string]bool)
	for _, p := range exclude {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if !strings.HasSuffix(p, "/") {
			p += "/"
		}
		excludeMap[p] = true
	}

	hostDir := baseParsed.Host
	_ = os.MkdirAll(hostDir, 0755)

	var visited sync.Map
	var wg sync.WaitGroup

	var crawl func(string)

	crawl = func(currentURL string) {
		defer wg.Done()

		if _, seen := visited.LoadOrStore(currentURL, true); seen {
			return
		}

		parsed, err := url.Parse(currentURL)
		if err != nil || parsed.Host != baseParsed.Host {
			return
		}

		filePath := parsed.Path
		if filePath == "" || filePath == "/" {
			filePath = "/index.html"
		}

		savePath := filepath.Join(hostDir, filePath)
		if strings.HasSuffix(parsed.Path, "/") {
			savePath = filepath.Join(savePath, "index.html")
		}

		if ext := filepath.Ext(savePath); rejectMap[ext] {
			return
		}

		for excl := range excludeMap {
			if strings.HasPrefix(parsed.Path+"/", excl) {
				return
			}
		}

		_ = os.MkdirAll(filepath.Dir(savePath), 0755)

		resp, err := http.Get(currentURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		f, err := os.Create(savePath)
		if err != nil {
			return
		}
		_, _ = f.Write(bodyBytes)
		f.Close()

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "text/css") {
			re := regexp.MustCompile(`(?:href|src|url)\s*=\s*["']([^"']+)["']`)
			for _, m := range re.FindAllSubmatch(bodyBytes, -1) {
				link := string(m[1])
				absURL := resolveURL(currentURL, link)
				if absURL != "" {
					wg.Add(1)
					go crawl(absURL) 
				}
			}
		}
	}

	wg.Add(1)
	go crawl(baseURL)
	wg.Wait()

	fmt.Fprintf(d.outWriter, "Mirroring completed for %s\n", baseURL)
}


// ====================== Helpers ======================
func resolveURL(base, ref string) string {
	b, _ := url.Parse(base)
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	resolved := b.ResolveReference(r)
	if resolved.Scheme == "" {
		return ""
	}
	return resolved.String()
}

func readLines(filename string) []string {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseRateLimit(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := int64(1)
	if strings.HasSuffix(s, "k") {
		mult = 1024
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "m") {
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n * mult
}

func humanSize(bytes int64) string {
	if bytes < 1024*1024 {
		return fmt.Sprintf("~%.2fKB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("~%.2fMB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("~%.2fGB", float64(bytes)/(1024*1024*1024))
}

// ====================== Progress Bar ======================
type progressWriter struct {
	total      int64
	downloaded int64
	writer     io.Writer
	start      time.Time
	last       time.Time
	mu         sync.Mutex
}

func newProgress(total int64, w io.Writer) *progressWriter {
	return &progressWriter{
		total:  total,
		writer: w,
		start:  time.Now(),
		last:   time.Now(),
	}
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	atomic.AddInt64(&p.downloaded, int64(n))

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if now.Sub(p.last) < 150*time.Millisecond && p.downloaded < p.total {
		return n, nil
	}
	p.last = now

	down := atomic.LoadInt64(&p.downloaded)
	perc := 100.0 * float64(down) / float64(p.total)
	elapsed := now.Sub(p.start).Seconds()
	speed := float64(down) / elapsed / (1024 * 1024)

	remaining := "?"
	if speed > 0 {
		left := (float64(p.total) - float64(down)) / (speed * 1024 * 1024)
		if left < 1 {
			remaining = "<1s"
		} else {
			remaining = fmt.Sprintf("%.0fs", left)
		}
	}

	downStr, totalStr := formatProgressSize(down, p.total)

	barLen := 50
	filled := int(perc / 2)
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", barLen-filled)

	fmt.Fprintf(p.writer,
		"\r %s / %s [%s] %.2f%% %.2f MiB/s %s",
		downStr, totalStr, bar, perc, speed, remaining,
	)

	return n, nil
}

func (p *progressWriter) finish() {
	fmt.Fprintln(p.writer)
}

func formatProgressSize(downloaded, total int64) (string, string) {
	if total > 10*1024*1024 {
		return fmt.Sprintf("%.2f MiB", float64(downloaded)/(1024*1024)),
			fmt.Sprintf("%.2f MiB", float64(total)/(1024*1024))
	}
	return fmt.Sprintf("%.2f KiB", float64(downloaded)/1024),
		fmt.Sprintf("%.2f KiB", float64(total)/1024)
}

// ====================== Rate Limiter ======================
type rateLimitedReadCloser struct {
	rc    io.ReadCloser
	limit int64
	start time.Time
	read  int64
	mu    sync.Mutex
}

func newRateLimitedReader(rc io.ReadCloser, limit int64) io.ReadCloser {
	return &rateLimitedReadCloser{
		rc:    rc,
		limit: limit,
		start: time.Now(),
	}
}

func (rl *rateLimitedReadCloser) Read(p []byte) (int, error) {
	n, err := rl.rc.Read(p)
	if n > 0 {
		rl.mu.Lock()
		rl.read += int64(n)
		elapsed := time.Since(rl.start).Seconds()
		allowed := int64(float64(rl.limit) * elapsed)
		if rl.read > allowed && rl.limit > 0 {
			sleepTime := time.Duration(float64(rl.read-allowed)/float64(rl.limit) * float64(time.Second))
			rl.mu.Unlock()
			time.Sleep(sleepTime)
			rl.mu.Lock()
		}
		rl.mu.Unlock()
	}
	return n, err
}

func (rl *rateLimitedReadCloser) Close() error {
	return rl.rc.Close()
}