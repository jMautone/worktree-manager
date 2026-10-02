package shell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDirectiveReplacesTheContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wt.directive")
	// Longer than the destination, so a write without truncation would
	// leave a tail behind.
	if err := os.WriteFile(path, []byte("/a/much/longer/path/than/the/destination\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "repo.worktrees", "feat")
	if err := WriteDirective(path, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != dest {
		t.Errorf("directive = %q, want exactly %q", got, dest)
	}
}

func TestWriteDirectiveToEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wt.directive")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const dest = `C:\src\repo.worktrees\feat`
	if err := WriteDirective(path, dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != dest {
		t.Errorf("directive = %q, want exactly %q", got, dest)
	}
}

func TestWriteDirectiveDoesNotCreateTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if err := WriteDirective(path, "/x"); err == nil {
		t.Fatal("WriteDirective to a missing file succeeded")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file exists at %s after the failure (stat: %v)", path, err)
	}
}
