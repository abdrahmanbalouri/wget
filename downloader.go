package main

import (
	"errors"
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

type downloadResult struct {
	index int
	urt64l   string
	size  in
	err   error
}

func (d *Downloader) downloadSingle(rawURL string) {
	_, err := d.downloadFile(rawURL, d.saveName, true)
	if err != nil {
		fmt.Fprintln(d.errWriter, err)
		return
	}

	fmt.Fprintf(d.outWriter, "\nDownloaded [%s]\n", rawURL)
	fmt.Fprintf(d.outWriter, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func (d *Downloader) downloadMultiple(urls []string) {
	var wg sync.WaitGroup
	results := make(chan downloadResult, len(urls))

	for i, rawURL := range urls {
		wg.Add(1)
		go func(i int, rawURL string) {
			defer wg.Done()
			tmp := *d
			tmp.saveName = ""
			size, err := tmp.downloadFile(rawURL, "", false)
			results <- downloadResult{index: i, url: rawURL, size: size, err: err}
		}(i, rawURL)
	}

	wg.Wait()
	close(results)

	sizes := make([]int64, len(urls))
	names := make([]string, len(urls))
	for res := range results {
		if res.err != nil {
			fmt.Fprintln(d.errWriter, res.err)
			continue
		}
		sizes[res.index] = res.size
		names[res.index] = fileNameFromRawURL(res.url)
	}

	fmt.Fprintf(d.outWriter, "content size: %v\n", sizes)
	for _, name := range names {
		if name != "" {
			fmt.Fprintf(d.outWriter, "finished %s\n", name)
		}
	}
	fmt.Fprintf(d.outWriter, "\nDownload finished:  %v\n", urls)
}

func (d *Downloader) downloadFile(rawURL, saveName string, showProgress bool) (int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, fmt.Errorf("invalid url")
	}

	switch u.Scheme {
	case "http", "https":
		return d.downloadHTTP(rawURL, saveName, showProgress)
	case "ftp":
		return d.downloadFTP(u, saveName, showProgress)
	default:
		return 0, fmt.Errorf("unsupported protocol: %s", u.Scheme)
	}
}

func (d *Downloader) downloadHTTP(rawURL, saveName string, showProgress bool) (int64, error) {
	u, _ := url.Parse(rawURL)
	if d.client == nil {
		d.client = &http.Client{}
	}

	headResp, err := d.client.Head(rawURL)
	if err != nil {
		return 0, err
	}
	if headResp.StatusCode != http.StatusOK && headResp.StatusCode != http.StatusMethodNotAllowed && headResp.StatusCode != http.StatusNotImplemented {
		status := headResp.Status
		headResp.Body.Close()
		return 0, fmt.Errorf("sending request, awaiting response... status %s", status)
	}

	size := headResp.ContentLength
	status := headResp.Status
	headResp.Body.Close()

	resp, err := d.client.Get(rawURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("sending request, awaiting response... status %s", resp.Status)
	}
	if size < 0 {
		size = resp.ContentLength
	}
	if status == "" || status == "405 Method Not Allowed" || status == "501 Not Implemented" {
		status = resp.Status
	}

	fmt.Fprintf(d.outWriter, "sending request, awaiting response... status %s\n", status)
	return d.saveResponse(u, saveName, size, resp.Body, showProgress)
}

func (d *Downloader) downloadFTP(u *url.URL, saveName string, showProgress bool) (int64, error) {
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":21"
	}

	client, err := newFTPClient(host)
	if err != nil {
		return 0, err
	}
	defer client.Close()

	user := "anonymous"
	pass := "anonymous"
	if u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	}

	if err := client.Login(user, pass); err != nil {
		return 0, err
	}

	size, err := client.Size(u.Path)
	if err != nil {
		var statusErr *ftpStatusError
		if errors.As(err, &statusErr) && statusErr.op == "size" && statusErr.code == 550 {
			fmt.Fprintln(d.outWriter, "sending request, awaiting response... status 200 OK")
			reader, listErr := client.List(u.Path)
			if listErr != nil {
				return 0, listErr
			}
			defer reader.Close()

			return d.saveResponse(u, saveName, 0, reader, false)
		}
		return 0, err
	}

	fmt.Fprintln(d.outWriter, "sending request, awaiting response... status 200 OK")
	reader, err := client.Retrieve(u.Path)
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	return d.saveResponse(u, saveName, size, reader, showProgress)
}

func (d *Downloader) saveResponse(u *url.URL, saveName string, size int64, body io.Reader, showProgress bool) (int64, error) {
	name := saveName
	if name == "" {
		name = fileNameFromURL(u)
	}
	savePath := filepath.Join(d.saveDir, name)
	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		return 0, err
	}

	fmt.Fprintf(d.outWriter, "content size: %d [%s]\n", size, approxMB(size))
	fmt.Fprintf(d.outWriter, "saving file to: %s\n", savePath)

	reader := body
	if d.rateLimit > 0 {
		if rc, ok := body.(io.ReadCloser); ok {
			reader = newRateLimitedReader(rc, d.rateLimit)
		}
	}

	file, err := os.Create(savePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	if showProgress {
		progress := newProgress(size, d.outWriter)
		_, err = io.Copy(io.MultiWriter(file, progress), reader)
		progress.finish()
		return size, err
	}

	_, err = io.Copy(file, reader)
	return size, err
}

func fileNameFromURL(u *url.URL) string {
	name := path.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		return "index.html"
	}
	return name
}

func fileNameFromRawURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return fileNameFromURL(u)
}
