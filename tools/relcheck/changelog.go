package main

import "strings"

// ChangelogSection returns the body of the "## [X.Y.Z]" section of a Keep a
// Changelog file, without its heading and without a trailing "---" separator.
// ok is false when the section does not exist or is empty: a final needs
// release notes.
func ChangelogSection(content string, v Version) (body string, ok bool) {
	heading := "## [" + strings.TrimPrefix(v.String(), "v") + "]"
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start, end := -1, len(lines)
	for i, l := range lines {
		if start < 0 {
			if l == heading || strings.HasPrefix(l, heading+" ") {
				start = i + 1
			}
			continue
		}
		if strings.HasPrefix(l, "## ") {
			end = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	body = strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	body = strings.TrimSpace(strings.TrimSuffix(body, "---"))
	return body, body != ""
}
