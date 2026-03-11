package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func extractLinksFromHTML(body []byte) []string {
	re := regexp.MustCompile(`(?is)<(?:a|link)\b[^>]*\bhref=["']([^"']+)["']|<(?:img)\b[^>]*\bsrc=["']([^"']+)["']`)
	matches := re.FindAllSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		switch {
		case len(match) > 1 && len(match[1]) > 0:
			links = append(links, string(match[1]))
		case len(match) > 2 && len(match[2]) > 0:
			links = append(links, string(match[2]))
		}
	}
	return links
}

func extractCSSLinks(body []byte) []string {
	re := regexp.MustCompile(`url\((['"]?)([^'")]+)\1\)`)
	matches := re.FindAllSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 2 {
			links = append(links, string(match[2]))
		}
	}
	return links
}

func convertHTMLLinks(path string, rewrites map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	content := string(data)
	for original, local := range rewrites {
		local = filepath.ToSlash(local)
		content = strings.ReplaceAll(content, `href="`+original+`"`, `href="`+local+`"`)
		content = strings.ReplaceAll(content, `href='`+original+`'`, `href='`+local+`'`)
		content = strings.ReplaceAll(content, `src="`+original+`"`, `src="`+local+`"`)
		content = strings.ReplaceAll(content, `src='`+original+`'`, `src='`+local+`'`)
	}

	_ = os.WriteFile(path, []byte(content), 0o644)
}

func convertCSSLinks(path string, rewrites map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	for original, local := range rewrites {
		content = strings.ReplaceAll(content, original, filepath.ToSlash(local))
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}
