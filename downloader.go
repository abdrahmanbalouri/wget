package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

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

func (d *Downloader) downloadSingle(rawURL string) {
	if d.client == nil {
		d.client = &http.Client{}
	}

	resp, err := d.client.Head(rawURL)
	if err != nil || resp.StatusCode != 200 {
		fmt.Fprintln(d.errWriter, "request failed")
		return
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
