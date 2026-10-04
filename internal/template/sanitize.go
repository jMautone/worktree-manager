package template

import "strings"

// reserved are the device names of Windows. A file or directory cannot have
// one of them as its name, or as its name before the first dot, ignoring
// case.
var reserved = []string{
	"CON", "PRN", "AUX", "NUL",
	"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
}

// Reserved reports whether name, or its part before the first dot, is a
// reserved device name of Windows, ignoring case: nul, Con.txt, lpt9.tar.gz.
func Reserved(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	for _, r := range reserved {
		if strings.EqualFold(stem, r) {
			return true
		}
	}
	return false
}

// Invalid reports whether r may not appear in a file name on Windows: a
// separator, one of < > : " | ? *, or a control character. The same set is
// what Sanitize replaces.
func Invalid(r rune) bool {
	return r < 0x20 || r == 0x7f || strings.ContainsRune(`/\<>:"|?*`, r)
}

// Sanitize turns s into a name usable as one directory name on macOS,
// Windows and Linux. It does not depend on the OS: the same branch gives the
// same directory on every machine. The rules, in order:
//
//  1. every character Invalid reports is replaced by -;
//  2. every . and space at the end is replaced by -;
//  3. when s, or its part before the first dot, is Reserved, a - is
//     inserted right after that part: nul -> nul-, Con.txt -> Con-.txt.
//
// Every other character is kept.
func Sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if Invalid(r) {
			return '-'
		}
		return r
	}, s)
	trimmed := strings.TrimRight(s, ". ")
	s = trimmed + strings.Repeat("-", len(s)-len(trimmed))
	if Reserved(s) {
		stem, rest, dot := strings.Cut(s, ".")
		s = stem + "-"
		if dot {
			s += "." + rest
		}
	}
	return s
}
