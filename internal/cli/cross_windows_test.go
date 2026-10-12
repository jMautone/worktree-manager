package cli_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Scenarios of Windows: "Backslash on Windows" and "Forward slashes on
// Windows" of cross-repo-resolution, and "Native path on Windows" of
// navigate. They run only there; the pure rules are also tested with goos
// windows on every OS.
func TestCrossRepoOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	h := newHarness(t)
	repos, git := h.mkdir(h.sb.Path("Repos")), h.mkdir(h.sb.Path("GIT"))
	h.sb.InitRepo(filepath.Join(repos, "api"))
	api := filepath.Join(git, "api")
	h.sb.InitRepo(api)
	feat := filepath.Join(git, "api.worktrees", "feat")
	h.sb.AddWorktree(api, feat, "feat")
	h.roots(0, repos, git)

	t.Run("backslash", func(t *testing.T) {
		mustCd(t, h, `GIT\api:`, api)
		mustCd(t, h, `GIT\api:feat`, feat)
		mustCd(t, h, "GIT/api:", api)
	})
	t.Run("forward slashes", func(t *testing.T) {
		r := mustFail(t, h, 4, `wt: "api" matches more than one repository`, "cd", "api")
		mustHint(t, r, "Repos/api:", filepath.Join(repos, "api"))
		mustHint(t, r, "GIT/api:", api)
		rows, _ := h.listAll()
		if got := column(rows, "REPO"); len(got) != 3 || got[0] != "GIT/api" || got[2] != "Repos/api" {
			t.Errorf("REPO = %q, want GIT/api and Repos/api", got)
		}
	})
	t.Run("native path", func(t *testing.T) {
		r := h.run("path", "GIT/api:feat")
		r.mustCode(t, 0)
		got := strings.TrimSuffix(r.stdout, "\n")
		if strings.Contains(got, "/") || !filepath.IsAbs(got) || !strings.HasSuffix(r.stdout, "\n") {
			t.Errorf("stdout = %q, want a native path and a newline", r.stdout)
		}
		mustPath(t, h, feat, "GIT/api:feat")
	})
}
