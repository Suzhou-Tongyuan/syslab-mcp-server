package tydocs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isDocFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".md", ".txt":
		return true
	default:
		return false
	}
}

func readDocText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	text := string(data)
	lowerPath := strings.ToLower(path)
	if strings.HasSuffix(lowerPath, ".html") || strings.HasSuffix(lowerPath, ".htm") {
		text = stripHTML(text)
	}
	return compactText(text), nil
}

func compactText(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = whitespacePattern.ReplaceAllString(strings.TrimSpace(line), " ")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var (
	tagPattern        = regexp.MustCompile(`(?s)<[^>]*>`)
	whitespacePattern = regexp.MustCompile(`\s+`)
)

func stripHTML(text string) string {
	text = tagPattern.ReplaceAllString(text, "\n")
	text = strings.NewReplacer(
		"&nbsp;", " ",
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
		"&quot;", `"`,
	).Replace(text)
	return text
}

func queryTokens(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	parts := strings.FieldsFunc(query, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		case r >= 0x4e00 && r <= 0x9fff:
			return false
		default:
			return true
		}
	})

	stopwords := map[string]struct{}{
		"use": {}, "using": {}, "with": {}, "from": {}, "the": {}, "and": {}, "for": {}, "how": {},
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if len([]rune(part)) < 2 {
			continue
		}
		if _, ok := stopwords[part]; ok {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}
