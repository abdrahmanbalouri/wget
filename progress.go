package main

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

type progressWriter struct {
	total int64
	done  int64
	start time.Time
	w     io.Writer
}

func newProgress(total int64, w io.Writer) *progressWriter {
	return &progressWriter{
		total: total,
		start: time.Now(),
		w:     w,
	}
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	atomic.AddInt64(&p.done, int64(n))

	elapsed := time.Since(p.start).Seconds()
	speed := float64(p.done) / elapsed // bytes/s
	percent := float64(p.done) * 100 / float64(p.total)

	barWidth := 100
	filled := int(percent)
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)

	fmt.Fprintf(
		p.w,
		"\r %s / %s [%s] %6.2f%% %s/s %ds",
		humanSize(p.done),
		humanSize(p.total),
		bar,
		percent,
		humanSize(int64(speed)),
		int(elapsed),
	)

	return n, nil
}

func (p *progressWriter) finish() {
	fmt.Fprintln(p.w)
}
