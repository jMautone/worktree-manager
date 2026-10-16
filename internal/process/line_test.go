package process

import "testing"

func TestLine(t *testing.T) {
	for _, tc := range []struct {
		goos  string
		words []string
		want  string
	}{
		// One word: the command line as is.
		{"darwin", []string{"echo a && echo b"}, "echo a && echo b"},
		{"linux", []string{"echo 100% $HOME"}, "echo 100% $HOME"},
		{"windows", []string{"echo a& echo b"}, "echo a& echo b"},
		{"windows", []string{"echo %PATH%"}, "echo %PATH%"},

		// macOS and Linux, several words.
		{"darwin", []string{"git", "-m", "--format=%h", "a/b.c"}, "git -m --format=%h a/b.c"},
		{"linux", []string{"p", "fix bug", "a;b", "$HOME", "*.go", "~", ""}, `p 'fix bug' 'a;b' '$HOME' '*.go' '~' ''`},
		{"linux", []string{"p", "it's"}, `p 'it'\''s'`},
		{"darwin", []string{"git", "commit", "-m", "fix bug"}, "git commit -m 'fix bug'"},

		// Windows, several words.
		{"windows", []string{"git", "commit", "-m", "fix bug"}, `git commit -m "fix bug"`},
		{"windows", []string{"p", "a&b", "(x86)"}, `p "a&b" "(x86)"`},
		{"windows", []string{"p", ""}, `p ""`},
		{"windows", []string{"p", `say "hi"`}, `p "say \"hi\""`},
		{"windows", []string{"p", `C:\dir x\`}, `p "C:\dir x\\"`},
		{"windows", []string{"p", `C:\dir\`}, `p C:\dir\`},
		{"windows", []string{"p", `a\"b`}, `p "a\\\"b"`},
		{"windows", []string{"p", `a "&" b`}, `p "a \"^&\" b"`},
		{"windows", []string{"p", "%WT_TEST%"}, "p %WT_TEST%"},
	} {
		if got := Line(tc.goos, tc.words); got != tc.want {
			t.Errorf("Line(%s, %q) = %s, want %s", tc.goos, tc.words, got, tc.want)
		}
	}
}

// Each word written by msvcrtQuote reads back, as the Microsoft C runtime
// splits a command line, as that word alone.
func TestMsvcrtQuoteRoundTrip(t *testing.T) {
	for _, w := range []string{"", "a", "a b", "\t", `"`, `""`, `\`, `\\`, `a\`, `a\\`, `a b\`, `a b\\`, `\"`, `\\"`, `a\"b`, `C:\Program Files\x\`, "a&b", "^", `"a" "b"`} {
		if got := msvcrtSplit(msvcrtQuote(w)); len(got) != 1 || got[0] != w {
			t.Errorf("msvcrtQuote(%q) = %s, read back as %q", w, msvcrtQuote(w), got)
		}
	}
}

// msvcrtSplit splits a command line as the Microsoft C runtime does: 2n
// backslashes and a " are n backslashes and a quote that toggles quoting;
// 2n+1 backslashes and a " are n backslashes and a literal "; other
// backslashes are literal; spaces and tabs outside quotes separate.
func msvcrtSplit(line string) []string {
	var args []string
	var cur []byte
	inArg, quoted := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\':
			n := 0
			for i < len(line) && line[i] == '\\' {
				n++
				i++
			}
			if i < len(line) && line[i] == '"' {
				for range n / 2 {
					cur = append(cur, '\\')
				}
				if n%2 == 1 {
					cur = append(cur, '"')
				} else {
					quoted = !quoted
				}
			} else {
				for range n {
					cur = append(cur, '\\')
				}
				i--
			}
			inArg = true
		case c == '"':
			quoted = !quoted
			inArg = true
		case (c == ' ' || c == '\t') && !quoted:
			if inArg {
				args = append(args, string(cur))
				cur, inArg = nil, false
			}
		default:
			cur = append(cur, c)
			inArg = true
		}
	}
	if inArg {
		args = append(args, string(cur))
	}
	return args
}
