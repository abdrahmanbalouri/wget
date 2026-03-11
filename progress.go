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
	if elapsed <= 0 {
		elapsed = 0.001
	}

	done := atomic.LoadInt64(&p.done)
	speed := float64(done) / elapsed

	percent := 0.0
	if p.total > 0 {
		percent = float64(done) * 100 / float64(p.total)
	}

	barWidth := 100
	filled := int((percent / 100) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)
	remaining := 0
	if p.total > 0 && speed > 0 {
		remaining = int(float64(p.total-done) / speed)
	}

	fmt.Fprintf(
		p.w,
		"\r %s / %s [%s] %6.2f%% %s/s %ds",
		humanSize(done),
		humanSize(p.total),
		bar,
		percent,
		humanSize(int64(speed)),
		remaining,
	)

	return n, nil
}

func (p *progressWriter) finish() {
	fmt.Fprintln(p.w)
}
