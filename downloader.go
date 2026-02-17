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

	"github.com/jlaffaye/ftp"
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
	u, err := url.Parse(rawURL)
	if err != nil {
		fmt.Fprintln(d.errWriter, "invalid url")
		return
	}

	switch u.Scheme {

	// =========================
	// HTTP / HTTPS
	// =========================
	case "http", "https":

		if d.client == nil {
			d.client = &http.Client{}
		}

		resp, err := d.client.Head(rawURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			fmt.Fprintln(d.errWriter, "request failed")
			return
		}

		size := resp.ContentLength
		resp.Body.Close()

		fmt.Fprintf(d.outWriter,
			"sending request, awaiting response... status 200 OK\n")
		fmt.Fprintf(d.outWriter,
			"content size: %d [%s]\n", size, approxMB(size))

		name := d.saveName
		if name == "" {
			name = path.Base(u.Path)
		}

		savePath := filepath.Join(d.saveDir, name)
		os.MkdirAll(filepath.Dir(savePath), 0o755)

		fmt.Fprintf(d.outWriter, "saving file to: %s\n", savePath)

		r, err := d.client.Get(rawURL)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		defer r.Body.Close()

		reader := r.Body
		if d.rateLimit > 0 {
			reader = newRateLimitedReader(r.Body, d.rateLimit)
		}

		f, err := os.Create(savePath)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		defer f.Close()

		p := newProgress(size, d.outWriter)
		io.Copy(io.MultiWriter(f, p), reader)
		p.finish()

		fmt.Fprintf(d.outWriter, "\nDownloaded [%s]\n", rawURL)
		fmt.Fprintf(d.outWriter,
			"finished at %s\n",
			time.Now().Format("2006-01-02 15:04:05"))

	// =========================
	// FTP
	// =========================
	case "ftp":

		host := u.Host
		if !strings.Contains(host, ":") {
			host += ":21"
		}

		c, err := ftp.Dial(host)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		defer c.Quit()

		user := "anonymous"
		pass := "anonymous"

		if u.User != nil {
			user = u.User.Username()
			if p, ok := u.User.Password(); ok {
				pass = p
			}
		}

		if err := c.Login(user, pass); err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}

		size, err := c.FileSize(u.Path)
		if err != nil {
			fmt.Println("dsfsdfs")
			fmt.Fprintln(d.errWriter, err)
			return
		}

		fmt.Fprintf(d.outWriter,
			"sending FTP request... 200 OK\n")
		fmt.Fprintf(d.outWriter,
			"content size: %d [%s]\n", size, approxMB(size))

		name := d.saveName
		if name == "" {
			name = path.Base(u.Path)
		}

		savePath := filepath.Join(d.saveDir, name)
		os.MkdirAll(filepath.Dir(savePath), 0o755)

		fmt.Fprintf(d.outWriter, "saving file to: %s\n", savePath)

		r, err := c.Retr(u.Path)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		defer r.Close()

		var reader io.ReadCloser = r
		if d.rateLimit > 0 {
			reader = newRateLimitedReader(r, d.rateLimit)
		}

		f, err := os.Create(savePath)
		if err != nil {
			fmt.Fprintln(d.errWriter, err)
			return
		}
		defer f.Close()

		p := newProgress(size, d.outWriter)
		io.Copy(io.MultiWriter(f, p), reader)
		p.finish()

		fmt.Fprintf(d.outWriter, "\nFTP Downloaded [%s]\n", rawURL)
		fmt.Fprintf(d.outWriter,
			"finished at %s\n",
			time.Now().Format("2006-01-02 15:04:05"))

	// =========================
	// Unsupported
	// =========================
	default:
		fmt.Fprintln(d.errWriter,
			"unsupported protocol:", u.Scheme)
	}
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
