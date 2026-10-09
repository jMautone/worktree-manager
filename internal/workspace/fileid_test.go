package workspace_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
	"github.com/jMautone/worktree-manager/internal/workspace"
)

func mustID(t *testing.T, path string) workspace.FileID {
	t.Helper()
	id, err := workspace.IDOf(path)
	if err != nil {
		t.Fatal(err)
	}
	if id == (workspace.FileID{}) {
		t.Fatalf("IDOf(%s) is the zero identity", path)
	}
	return id
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIDOfTellsDirectoriesApart(t *testing.T) {
	sb := testutil.New(t)
	a, b := mkdir(t, sb.Path("a")), mkdir(t, sb.Path("b"))
	if mustID(t, a) == mustID(t, b) {
		t.Error("two directories have the same identity")
	}
	if mustID(t, a) != mustID(t, filepath.Join(sb.Path("b"), "..", "a")) {
		t.Error("the same directory through .. has another identity")
	}
	if _, err := workspace.IDOf(sb.Path("missing")); err == nil {
		t.Error("IDOf of a missing directory: no error")
	}
}

// A symbolic link on macOS and Linux, and a junction on Windows, give the
// identity of their target.
func TestIDOfFollowsLinks(t *testing.T) {
	sb := testutil.New(t)
	target := mkdir(t, sb.Path("elsewhere", "frontend"))
	link := sb.Path("web")
	if runtime.GOOS == "windows" {
		sb.Junction(target, link)
	} else {
		sb.Symlink(target, link)
	}
	if mustID(t, link) != mustID(t, target) {
		t.Errorf("%s and its target %s have different identities", link, target)
	}
}

func TestIDOfIgnoresLetterCaseOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS, case-insensitive by default, is macOS's")
	}
	sb := testutil.New(t)
	dir := mkdir(t, sb.Path("TrendFisher"))
	other := filepath.Join(filepath.Dir(dir), strings.ToLower(filepath.Base(dir)))
	if _, err := os.Stat(other); err != nil {
		t.Skipf("the temporary directory is on a case-sensitive volume: %v", err)
	}
	if mustID(t, other) != mustID(t, dir) {
		t.Errorf("%s and %s have different identities", other, dir)
	}
}
