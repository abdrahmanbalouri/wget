package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

func readLines(file string) []string {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseRateLimit(s string) (int64, error) {
	s = strings.ToLower(s)
	if s == "" {
		return 0, nil
	}

	m := int64(1)
	if strings.HasSuffix(s, "k") {
		m = 1024
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "m") {
		m = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid rate limit, use values like 200k or 2M")
	}
	return n * m, nil
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
	if n <= 0 {
		return "unknown"
	}
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

func expandPath(p string) string {
	if p == "" {
		return "."
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		usr, err := user.Current()
		if err == nil {
			if p == "~" {
				return usr.HomeDir
			}
			return filepath.Join(usr.HomeDir, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}
