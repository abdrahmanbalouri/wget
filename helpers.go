package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

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

func humanSize(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)

	switch {
	case n >= MB:
		return fmt.Sprintf("%.2f MiB", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.2f KiB", float64(n)/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
func approxMB(n int64) string {
	mb := float64(n) / (1000 * 1000) // wget-style
	return fmt.Sprintf("~%.2fMB", mb)
}



func resolveURL(base, ref string) string {
	b, _ := url.Parse(base)
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return b.ResolveReference(r).String()
}
