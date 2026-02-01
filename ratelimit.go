package main

import (
	"io"
	"time"
)

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
