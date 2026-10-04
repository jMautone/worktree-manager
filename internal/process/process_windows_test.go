package process

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, cmdline string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code, err := Run(t.TempDir(), cmdline, os.Environ(), strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run(%q): %v", cmdline, err)
	}
	return code, strings.ReplaceAll(stdout.String(), "\r\n", "\n")
}

func TestRunWindows(t *testing.T) {
	if code, _ := run(t, "exit /b 3"); code != 3 {
		t.Errorf("exit /b 3: code %d", code)
	}
	if code, out := run(t, "echo a& echo b"); code != 0 || out != "a\nb\n" {
		t.Errorf("&: code %d, stdout %q", code, out)
	}
	// cmd prints the quotes, as when typed in cmd itself.
	if code, out := run(t, `echo "a b"`); code != 0 || out != "\"a b\"\n" {
		t.Errorf("quotes: code %d, stdout %q", code, out)
	}
}

func TestRunMissingDirectoryWindows(t *testing.T) {
	var out bytes.Buffer
	if _, err := Run(filepath.Join(t.TempDir(), "missing"), "exit /b 0", os.Environ(), nil, &out, &out); err == nil {
		t.Error("Run in a missing directory: no error")
	}
}
