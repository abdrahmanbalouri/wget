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

func humanSize(b int64) string {
	return fmt.Sprintf("~%.2fMB", float64(b)/(1024*1024))
}

func resolveURL(base, ref string) string {
	b, _ := url.Parse(base)
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return b.ResolveReference(r).String()
}
