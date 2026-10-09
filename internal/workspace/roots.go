package workspace

import (
	"fmt"
	"path"
	"strings"
)

// Root is one root of repos_root: Raw as written in the configuration, Path
// with ~ expanded and in goos's native, clean, absolute form.
type Root struct {
	Raw, Path string
}

// Roots expands the roots of repos_root for goos. home is the user's home
// directory (HOME, or USERPROFILE on windows). A root that needs ~ expanded
// while home is empty is skipped with a warning. Whether each root is
// absolute or starts with ~ was already checked by the configuration.
func Roots(raw []string, goos, home string) ([]Root, []string) {
	var roots []Root
	var warnings []string
	for _, r := range raw {
		p, ok := expandTilde(r, goos, home)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("cannot expand ~ in %s: %s is not set", r, homeVar(goos)))
			continue
		}
		roots = append(roots, Root{Raw: r, Path: clean(goos, p)})
	}
	return roots, warnings
}

// expandTilde replaces a leading ~ (alone, or followed by / or, on windows,
// by \) with home. It reports false when there is a ~ to expand and home is
// empty.
func expandTilde(p, goos, home string) (string, bool) {
	rest, ok := strings.CutPrefix(p, "~")
	if !ok || rest != "" && rest[0] != '/' && !(goos == "windows" && rest[0] == '\\') {
		return p, true
	}
	if home == "" {
		return "", false
	}
	if rest == "" {
		return home, true
	}
	return strings.TrimRight(home, `/\`) + rest, true
}

func homeVar(goos string) string {
	if goos == "windows" {
		return "USERPROFILE"
	}
	return "HOME"
}

// clean returns p in goos's native form, with . and .. resolved and repeated
// separators collapsed, as filepath.Clean does on goos. filepath would follow
// the OS running the code.
func clean(goos, p string) string {
	if goos != "windows" {
		return path.Clean(p)
	}
	p = strings.ReplaceAll(p, "/", `\`)
	vol := volume(p)
	rest := strings.ReplaceAll(p[len(vol):], `\`, "/")
	if strings.HasPrefix(vol, `\\`) {
		// The share of a UNC path is always followed by its root.
		rest = "/" + rest
	}
	return vol + strings.ReplaceAll(path.Clean(rest), "/", `\`)
}

// volume returns the volume of a windows path with backslashes: C: for a
// drive, \\server\share for a UNC path, or "".
func volume(p string) string {
	if len(p) >= 2 && isLetter(p[0]) && p[1] == ':' {
		return p[:2]
	}
	if rest, ok := strings.CutPrefix(p, `\\`); ok {
		server, after, ok := strings.Cut(rest, `\`)
		if !ok || server == "" {
			return p
		}
		share, _, _ := strings.Cut(after, `\`)
		return `\\` + server + `\` + share
	}
	return ""
}

// isAbs reports whether p is absolute on goos: it starts with / on macOS
// and Linux; on windows, it has a drive letter followed by \ or /, or is a
// UNC path.
func isAbs(goos, p string) bool {
	if goos != "windows" {
		return strings.HasPrefix(p, "/")
	}
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return true
	}
	return len(p) >= 3 && isLetter(p[0]) && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// join joins rel onto base with goos's separator and cleans the result.
func join(goos, base, rel string) string {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	return clean(goos, base+sep+rel)
}
