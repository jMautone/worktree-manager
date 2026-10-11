package cli_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// crossWorkspace creates under <sandbox>/root the repositories api, with
// the linked worktree <root>/api.worktrees/feat on the branch feat, and
// web, sets repos_root to the root, and starts outside any repository.
func crossWorkspace(h *harness) (root, feat string) {
	h.t.Helper()
	root = h.mkdir(h.sb.Path("root"))
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	h.sb.InitRepo(api)
	h.sb.InitRepo(web)
	feat = filepath.Join(root, "api.worktrees", "feat")
	h.sb.AddWorktree(api, feat, "feat")
	h.roots(0, root)
	return root, feat
}

// mustFail runs wt with args and checks the exit code and the first line of
// stderr, and that no directive was written. It returns the result.
func mustFail(t *testing.T, h *harness, code int, first string, args ...string) result {
	t.Helper()
	d := h.activate()
	r := h.run(args...)
	r.mustCode(t, code)
	if first != "" && firstLine(r.stderr) != first {
		t.Errorf("wt %v: first line = %q, want %q", args, firstLine(r.stderr), first)
	}
	if got := readDirective(t, d); got != "" || r.stdout != "" {
		t.Errorf("wt %v: stdout %q, directive %q; want both empty", args, r.stdout, got)
	}
	return r
}

func mustHint(t *testing.T, r result, parts ...string) {
	t.Helper()
	for _, line := range strings.Split(r.stderr, "\n") {
		if !strings.HasPrefix(line, "hint: ") {
			continue
		}
		all := true
		for _, p := range parts {
			all = all && strings.Contains(line, p)
		}
		if all {
			return
		}
	}
	t.Errorf("stderr has no hint line with %q:\n%s", parts, r.stderr)
}

// Scenarios of "Worktrees of other repositories" (cross-repo-resolution)
// and "Worktree of another repository" (navigate).
func TestCdWorktreeOfAnotherRepository(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	abc := filepath.Join(root, "api.worktrees", "feature-abc1")
	h.sb.AddWorktree(api, abc, "feature/abc1")

	t.Run("from another repository", func(t *testing.T) {
		h.cwd = web
		mustCd(t, h, "feat", feat)
	})
	t.Run("from outside any repository", func(t *testing.T) {
		h.cwd = h.sb.Path("outside")
		mustCd(t, h, "feat", feat)
	})
	t.Run("by branch", func(t *testing.T) {
		h.cwd = web
		mustCd(t, h, "feature/abc1", abc)
	})
	t.Run("name before branch", func(t *testing.T) {
		fix := filepath.Join(root, "api.worktrees", "fix")
		h.sb.AddWorktree(api, fix, "fix-1")
		h.sb.AddWorktree(web, filepath.Join(root, "web.worktrees", "x"), "fix")
		h.cwd = h.sb.Path("outside")
		mustCd(t, h, "fix", fix)
	})
	t.Run("main worktrees are not candidates", func(t *testing.T) {
		h.cwd = h.sb.Path("outside")
		mustFail(t, h, 3, `wt: no worktree or repository named "main"`, "cd", "main")
	})
	t.Run("a worktree of the current repository is not another's", func(t *testing.T) {
		// feat is a worktree of api: from inside api it resolves locally.
		h.cwd = api
		mustCd(t, h, "feat", feat)
	})
}

func TestCdSameWorktreeNameInTwoRepositories(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	webFeat := filepath.Join(root, "web.worktrees", "feat")
	h.sb.AddWorktree(filepath.Join(root, "web"), webFeat, "web-feat")

	r := mustFail(t, h, 4, `wt: "feat" matches more than one worktree`, "cd", "feat")
	mustHint(t, r, "api:feat", feat)
	mustHint(t, r, "web:feat", webFeat)
	if strings.Contains(r.stderr, "warning") {
		t.Errorf("stderr = %q, want no warning", r.stderr)
	}

	// The qualified form each hint names leads to its worktree.
	mustCd(t, h, "api:feat", feat)
	mustCd(t, h, "web:feat", webFeat)
}

// Scenarios "Repository before worktrees of other repositories" and
// "Ambiguous repositories stop the search".
func TestCdRepositoriesBeforeOtherWorktrees(t *testing.T) {
	h := newHarness(t)
	root, _ := crossWorkspace(h)
	api := filepath.Join(root, "api")
	for _, name := range []string{"feature-flags", "cv-scorer-backend", "cv-scorer-front"} {
		h.sb.InitRepo(filepath.Join(root, name))
	}
	h.sb.AddWorktree(api, filepath.Join(root, "api.worktrees", "cv"), "cv")

	h.cwd = filepath.Join(root, "web")
	mustCd(t, h, "feat", filepath.Join(root, "feature-flags"))

	h.cwd = h.sb.Path("outside")
	mustFail(t, h, 4, `wt: "cv" matches more than one repository`, "cd", "cv")
}

// Scenarios of "Qualified targets" and "Name side".
func TestCdQualified(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	abc := filepath.Join(root, "api.worktrees", "feature-abc1")
	h.sb.AddWorktree(api, abc, "feature/abc1")
	webFeat := filepath.Join(root, "web.worktrees", "feat")
	h.sb.AddWorktree(web, webFeat, "web-feat")
	webAPI := filepath.Join(root, "web.worktrees", "api")
	h.sb.AddWorktree(web, webAPI, "web-api")
	h.cwd = web

	for _, tc := range []struct {
		name, target, want string
	}{
		{"worktree of another repository", "api:feat", feat},
		{"same name as a local worktree", "api:feat", feat},
		{"local worktree, qualified", "web:feat", webFeat},
		{"by branch", "api:feature/abc1", abc},
		{"main worktree by its branch", "api:main", api},
		{"main worktree", "api:^", api},
		{"repository itself", "api:", api},
		{"current repository is not searched", "api:", api},
		{"repository by prefix", "ap:feat", feat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustCd(t, h, tc.target, tc.want)
		})
	}

	t.Run("usage errors", func(t *testing.T) {
		for _, target := range []string{":feat", ":", "api:@", "api:-"} {
			r := mustFail(t, h, 2, "", "cd", target)
			if !strings.Contains(firstLine(r.stderr), `"`+target+`"`) {
				t.Errorf("wt cd %s: first line %q does not name the target", target, firstLine(r.stderr))
			}
		}
	})
	t.Run("no such worktree in the repository", func(t *testing.T) {
		r := mustFail(t, h, 3, `wt: no worktree named "nope" in api`, "cd", "api:nope")
		mustHint(t, r, "wt list --all-repos")
	})
	t.Run("no such repository", func(t *testing.T) {
		r := mustFail(t, h, 3, `wt: no repository named "nope"`, "cd", "nope:feat")
		mustHint(t, r, "wt repos")
	})
	t.Run("ambiguous repository side", func(t *testing.T) {
		h.sb.InitRepo(filepath.Join(root, "apiary"))
		mustFail(t, h, 4, `wt: "ap" matches more than one repository`, "cd", "ap:feat")
	})
	t.Run("no roots configured", func(t *testing.T) {
		h.userConfig("")
		r := mustFail(t, h, 1, "wt: no repository roots configured", "cd", "api:feat")
		mustHint(t, r, "repos_root", h.userConfigPath())
		h.roots(0, root)
	})
}

func TestCdQualifiedNameWithAColon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a directory name cannot contain : on Windows")
	}
	h := newHarness(t)
	root, _ := crossWorkspace(h)
	ab := filepath.Join(root, "api.worktrees", "a:b")
	h.sb.AddWorktree(filepath.Join(root, "api"), ab, "ab")
	mustCd(t, h, "api:a:b", ab)
}

// Scenarios of "Repository side" with repositories of the same name.
func TestCdQualifiedPathSuffix(t *testing.T) {
	h := newHarness(t)
	home := h.sb.Getenv("HOME")
	if runtime.GOOS == "windows" {
		home = h.sb.Getenv("USERPROFILE")
	}
	repos, git := filepath.Join(home, "Repos"), filepath.Join(home, "Documents", "GIT")
	h.sb.InitRepo(filepath.Join(repos, "api"))
	h.sb.InitRepo(filepath.Join(git, "api"))
	feat := filepath.Join(git, "api.worktrees", "feat")
	h.sb.AddWorktree(filepath.Join(git, "api"), feat, "feat")
	h.userConfig(`repos_root = ["~/Repos", "~/Documents/GIT"]` + "\n")

	mustCd(t, h, "GIT/api:feat", feat)
	mustCd(t, h, "git/API:", filepath.Join(git, "api"))
	mustCd(t, h, "/Documents//GIT/api/:", filepath.Join(git, "api"))
	mustCd(t, h, "Repos/api:", filepath.Join(repos, "api"))

	mustFail(t, h, 3, `wt: no repository named "GIT/ap"`, "cd", "GIT/ap:")

	r := mustFail(t, h, 4, `wt: "api" matches more than one repository`, "cd", "api:feat")
	mustHint(t, r, "Repos/api:", filepath.Join(repos, "api"))
	mustHint(t, r, "GIT/api:", filepath.Join(git, "api"))

	// A repository named by its suffix is not ambiguous by its name.
	r = mustFail(t, h, 4, `wt: "api" matches more than one repository`, "cd", "api")
	mustHint(t, r, "GIT/api:", filepath.Join(git, "api"))

	if runtime.GOOS != "windows" {
		mustFail(t, h, 3, `wt: no repository named "GIT\api"`, "cd", `GIT\api:`)
	}
}

// Scenario "Repository itself in the .bare layout", for wt cd.
func TestCdQualifiedDotBare(t *testing.T) {
	h := newHarness(t)
	root := h.mkdir(h.sb.Path("root"))
	proj := filepath.Join(root, "proj")
	h.sb.InitDotBare(proj)
	h.roots(0, root)
	mustCd(t, h, "proj:", proj)
}

// Scenarios of "Repositories git cannot read".
func TestCdUnreadableRepository(t *testing.T) {
	h := newHarness(t)
	root, feat := crossWorkspace(h)
	broken := filepath.Join(root, "broken")
	h.sb.InitRepo(broken)
	h.sb.BreakHEAD(broken)

	t.Run("found elsewhere", func(t *testing.T) {
		d := h.activate()
		r := h.run("cd", "feat")
		r.mustCode(t, 0)
		if !strings.Contains(r.stderr, "wt: warning: cannot read the worktrees of broken: ") {
			t.Errorf("stderr = %q, want a warning naming broken", r.stderr)
		}
		mustDirective(t, d, feat)
	})
	t.Run("found nowhere", func(t *testing.T) {
		d := h.activate()
		r := h.run("cd", "nope")
		r.mustCode(t, 3)
		if !strings.Contains(r.stderr, "wt: warning: cannot read the worktrees of broken: ") ||
			!strings.Contains(r.stderr, "\nwt: no worktree or repository named \"nope\"\n") {
			t.Errorf("stderr = %q, want the warning, then the error", r.stderr)
		}
		if readDirective(t, d) != "" {
			t.Error("the directive was written")
		}
	})
	t.Run("not read when not needed", func(t *testing.T) {
		mustCd(t, h, "api", filepath.Join(root, "api"))
	})
	t.Run("named by a qualified target", func(t *testing.T) {
		r := mustFail(t, h, 1, "", "cd", "broken:feat")
		if !strings.Contains(r.stderr, "not a git repository") || strings.Contains(r.stderr, "warning") {
			t.Errorf("stderr = %q, want git's message and no warning", r.stderr)
		}
	})
}

// An invalid .wt.toml in the working directory is still reported when the
// target is resolved.
func TestCdInvalidRepositoryFile(t *testing.T) {
	h := newHarness(t)
	repo, _ := cdRepo(h)
	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \n")
	d := h.activate()
	r := h.run("cd", "feat")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, ".wt.toml") || readDirective(t, d) != "" {
		t.Errorf("stderr = %q, directive %q; want the file named and no directive", r.stderr, readDirective(t, d))
	}
}
