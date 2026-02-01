package main

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

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
