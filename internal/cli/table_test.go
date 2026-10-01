package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestRenderTableAlignsByVisibleWidth(t *testing.T) {
	rows := [][]cell{
		{{}, {text: "NAME", style: styleBold}, {text: "BRANCH", style: styleBold}, {text: "STATE", style: styleBold}, {text: "PATH", style: styleBold}},
		{{text: "@^", style: styleCurrent}, {text: "repo", style: styleBold}, {text: "main", style: styleBranch}, {}, {text: "/a/repo"}},
		{{}, {text: "a-much-longer-name"}, {text: "(detached)", style: styleWarn}, {text: "locked", style: styleWarn}, {text: "/a/b"}},
		{{text: "^"}, {text: "ñandú"}, {text: "x"}, {}, {text: "/a/ñ"}},
	}
	var colored, plain bytes.Buffer
	if err := renderTable(&colored, rows, true); err != nil {
		t.Fatal(err)
	}
	if err := renderTable(&plain, rows, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatal("colored table has no ANSI sequences")
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatal("plain table has ANSI sequences")
	}

	stripped := ansiSeq.ReplaceAllString(colored.String(), "")
	if stripped != plain.String() {
		t.Errorf("without ANSI the colored table differs from the plain one:\n%s\nvs\n%s", stripped, plain.String())
	}

	lines := strings.Split(strings.TrimSuffix(stripped, "\n"), "\n")
	if len(lines) != len(rows) {
		t.Fatalf("got %d lines, want %d", len(lines), len(rows))
	}
	for col := 1; col < len(rows[0]); col++ {
		offset := len([]rune(lines[0][:strings.Index(lines[0], rows[0][col].text)]))
		for i, line := range lines {
			runes := []rune(line)
			if want := rows[i][col].text; want != "" && !strings.HasPrefix(string(runes[offset:]), want) {
				t.Errorf("row %d: column %d does not start at offset %d: %q", i, col, offset, line)
			}
			if runes[offset-1] != ' ' {
				t.Errorf("row %d: no space before column %d: %q", i, col, line)
			}
		}
	}
	for _, line := range lines {
		if strings.TrimRight(line, " ") != line {
			t.Errorf("trailing whitespace in %q", line)
		}
	}
}
