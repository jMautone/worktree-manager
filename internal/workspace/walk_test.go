package workspace_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
	"github.com/jMautone/worktree-manager/internal/workspace"
)

// discover walks roots and lists what it found, as wt repos does.
func discover(t *testing.T, depth int, roots ...string) ([]workspace.Repo, []string) {
	t.Helper()
	var rs []workspace.Root
	for _, r := range roots {
		rs = append(rs, workspace.Root{Raw: r, Path: r})
	}
	found, warnings := workspace.Walk(rs, depth)
	return workspace.List(found, runtime.GOOS), warnings
}

// repoPaths returns the paths of repos, in order.
func repoPaths(repos []workspace.Repo) []string {
	ps := []string{}
	for _, r := range repos {
		ps = append(ps, r.Path)
	}
	return ps
}

func mustRepos(t *testing.T, got []workspace.Repo, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if ps := repoPaths(got); !reflect.DeepEqual(ps, want) {
		t.Errorf("repositories = %q, want %q", ps, want)
	}
}

func TestWalkKindsOfRepository(t *testing.T) {
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))

	clone := filepath.Join(root, "api")
	sb.InitRepo(clone)
	sb.AddWorktree(clone, filepath.Join(root, "api.worktrees", "feat"), "feat")
	proj := filepath.Join(root, "proj")
	sb.InitDotBare(proj)
	sep := filepath.Join(root, "sep")
	sb.InitSeparateGitDir(sep, sb.Path("store", "sep.git"))
	sb.InitBare(filepath.Join(root, "lone.git"))
	sb.WriteFile(filepath.Join(root, "notes", "todo.txt"), "x")
	sb.WriteFile(filepath.Join(root, "file.txt"), "not a directory")
	dotfiles := filepath.Join(root, ".dotfiles")
	sb.InitRepo(dotfiles)

	repos, warnings := discover(t, 2, root)
	mustRepos(t, repos, dotfiles, clone, proj, sep)
	if len(warnings) != 0 {
		t.Errorf("warnings = %q", warnings)
	}
	for _, r := range repos {
		if r.Root != root || r.Name != filepath.Base(r.Path) {
			t.Errorf("%s: root %q, name %q", r.Path, r.Root, r.Name)
		}
	}

	// The .bare layout is identified by its bare repository too, which is
	// what git reports as its main worktree.
	if got := workspace.Current(repos, mustID(t, filepath.Join(proj, ".bare"))); got < 0 || repos[got].Path != proj {
		t.Errorf("Current(proj/.bare) = %d, want the row of %s", got, proj)
	}
	if got := workspace.Current(repos, mustID(t, clone)); got < 0 || repos[got].Path != clone {
		t.Errorf("Current(api) = %d, want the row of %s", got, clone)
	}
}

func TestWalkDepth(t *testing.T) {
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))
	api := filepath.Join(root, "org", "api")
	sb.InitRepo(api)
	outer := filepath.Join(root, "outer")
	sb.InitRepo(outer)
	sb.InitRepo(filepath.Join(outer, "vendor", "lib"))
	deep := filepath.Join(root, "a", "b", "c")
	sb.InitRepo(deep)
	sb.InitRepo(filepath.Join(root, "a", "b", "c", "d", "too-deep"))
	sb.InitRepo(filepath.Join(root, "x", "y", "z", "four"))

	repos, _ := discover(t, 1, root)
	mustRepos(t, repos, outer)

	repos, _ = discover(t, 2, root)
	mustRepos(t, repos, api, outer)

	// The repository inside a repository is never found.
	repos, _ = discover(t, 3, root)
	mustRepos(t, repos, api, deep, outer)
}

func TestWalkRoots(t *testing.T) {
	sb := testutil.New(t)
	dir := mkdir(t, sb.Path("dir"))
	work := filepath.Join(dir, "work")
	api := filepath.Join(work, "api")
	sb.InitRepo(api)
	web := filepath.Join(dir, "web")
	sb.InitRepo(web)
	missing := sb.Path("usb", "repos")
	file := sb.Path("file")
	sb.WriteFile(file, "x")

	repos, warnings := discover(t, 2, missing, work, file, dir, work)
	// api is found under work first, so it keeps that root.
	mustRepos(t, repos, api, web)
	if repos[0].Root != work || repos[1].Root != dir {
		t.Errorf("roots = %q, %q; want %q, %q", repos[0].Root, repos[1].Root, work, dir)
	}
	want := []string{"repository root not found: " + missing, "repository root is not a directory: " + file}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("warnings = %q, want %q", warnings, want)
	}

	if repos, warnings := discover(t, 1); len(repos) != 0 || len(warnings) != 0 {
		t.Errorf("no roots: %v, %q", repos, warnings)
	}
}

// An orphaned worktree, whose repository was deleted, is neither a
// repository nor walked into.
func TestWalkOrphanedWorktree(t *testing.T) {
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))
	gone := sb.Path("gone")
	sb.InitRepo(gone)
	orphan := filepath.Join(root, "orphan")
	sb.AddWorktree(gone, orphan, "x")
	sb.InitRepo(filepath.Join(orphan, "inner"))
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	repos, warnings := discover(t, 2, root)
	mustRepos(t, repos)
	if len(warnings) != 0 {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestWalkFollowsSymbolicLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows; junctions are tested in wt repos")
	}
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))
	frontend := sb.Path("elsewhere", "frontend")
	sb.InitRepo(frontend)
	web := filepath.Join(root, "web")
	sb.Symlink(frontend, web)
	api := filepath.Join(root, "api")
	sb.InitRepo(api)
	sb.Symlink(api, filepath.Join(root, "zz-api"))
	sb.Symlink(sb.Path("nowhere"), filepath.Join(root, "dangling"))

	repos, warnings := discover(t, 3, root)
	mustRepos(t, repos, api, web)
	if len(warnings) != 0 {
		t.Errorf("warnings = %q", warnings)
	}
}

// A link back to the root is a cycle; the depth limit ends it, and each
// repository is listed once.
func TestWalkEndsInACycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows")
	}
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))
	sb.InitRepo(filepath.Join(root, "api"))
	sb.InitRepo(filepath.Join(root, "web"))
	sb.Symlink(root, filepath.Join(root, "loop"))

	repos, _ := discover(t, 3, root)
	if len(repos) != 2 || repos[0].Name != "api" || repos[1].Name != "web" {
		t.Errorf("repositories = %+v, want api and web once each", repos)
	}
}

func TestWalkSkipsUnreadableDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not work this way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))
	api := filepath.Join(root, "api")
	sb.InitRepo(api)
	locked := mkdir(t, filepath.Join(root, "locked", "inside"))
	locked = filepath.Dir(locked)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	repos, warnings := discover(t, 2, root)
	mustRepos(t, repos, api)
	if len(warnings) != 0 {
		t.Errorf("warnings = %q", warnings)
	}
}

// The walk works on native paths under a root given with a trailing
// separator, as a user may write it.
func TestWalkPathsAreClean(t *testing.T) {
	sb := testutil.New(t)
	root := mkdir(t, sb.Path("root"))
	sb.InitRepo(filepath.Join(root, "api"))
	repos, _ := discover(t, 1, root+string(filepath.Separator))
	if len(repos) != 1 || strings.Contains(repos[0].Path, string(filepath.Separator)+string(filepath.Separator)) {
		t.Errorf("repositories = %+v", repos)
	}
}
