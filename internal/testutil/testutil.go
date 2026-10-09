// Package testutil builds real git repositories in temporary directories for
// tests, with git and wt isolated from the configuration of whoever runs them.
//
// Tests run against real git, never mocks (see docs/design/product.md). Without
// the isolation, the user's global git config and wt config leak into results
// and tests pass or fail depending on the machine.
package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// gitconfig is the global git configuration every sandbox starts with.
const gitconfig = `[user]
	name = wt tests
	email = wt@example.com
[init]
	defaultBranch = main
[commit]
	gpgsign = false
[core]
	autocrlf = false
`

// Sandbox is a temporary directory with an isolated environment for git and
// wt. Every path it creates lives under Root.
type Sandbox struct {
	t    testing.TB
	Root string
	env  []string
}

// New creates a sandbox under t.TempDir(). The environment starts from the
// test process's (so PATH and the OS's own variables keep working) minus
// anything that configures git or wt, and points HOME, USERPROFILE,
// XDG_CONFIG_HOME and APPDATA inside the sandbox.
func New(t testing.TB) *Sandbox {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	appdata := filepath.Join(home, "AppData", "Roaming")
	for _, dir := range []string{home, appdata} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	globalConfig := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(globalConfig, []byte(gitconfig), 0o644); err != nil {
		t.Fatal(err)
	}

	sb := &Sandbox{t: t, Root: root}
	for _, kv := range os.Environ() {
		if !leaks(kv) {
			sb.env = append(sb.env, kv)
		}
	}
	sb.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	sb.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	// Stop repository discovery at the sandbox, in case the system temp
	// directory is itself inside a repository.
	sb.Setenv("GIT_CEILING_DIRECTORIES", root)
	sb.Setenv("HOME", home)
	sb.Setenv("USERPROFILE", home)
	sb.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	sb.Setenv("APPDATA", appdata)
	return sb
}

// leaks reports whether an inherited variable would let the host's git or wt
// configuration into the sandbox.
func leaks(kv string) bool {
	key, _, _ := strings.Cut(kv, "=")
	key = strings.ToUpper(key)
	if strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "WT_") {
		return true
	}
	switch key {
	case "HOME", "USERPROFILE", "XDG_CONFIG_HOME", "APPDATA", "NO_COLOR":
		return true
	}
	return false
}

// sameKey compares environment variable names the way the OS does.
func sameKey(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Setenv sets a variable in the sandbox environment, replacing any previous
// value. It does not touch the test process's environment.
func (s *Sandbox) Setenv(key, value string) {
	for i, kv := range s.env {
		if k, _, _ := strings.Cut(kv, "="); sameKey(k, key) {
			s.env[i] = key + "=" + value
			return
		}
	}
	s.env = append(s.env, key+"="+value)
}

// Getenv returns a variable from the sandbox environment, or "" if unset.
func (s *Sandbox) Getenv(key string) string {
	for _, kv := range s.env {
		if k, v, _ := strings.Cut(kv, "="); sameKey(k, key) {
			return v
		}
	}
	return ""
}

// Environ returns a copy of the sandbox environment in os.Environ form.
func (s *Sandbox) Environ() []string {
	return append([]string(nil), s.env...)
}

// Path joins elem onto the sandbox root.
func (s *Sandbox) Path(elem ...string) string {
	return filepath.Join(append([]string{s.Root}, elem...)...)
}

// WriteFile writes content to path, creating parent directories.
func (s *Sandbox) WriteFile(path, content string) {
	s.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// Git runs git in dir with the sandbox environment and returns its trimmed
// stdout. Any failure fails the test.
func (s *Sandbox) Git(dir string, args ...string) string {
	s.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = s.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		s.t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// InitRepo creates a repository at dir with one commit on main.
func (s *Sandbox) InitRepo(dir string) {
	s.t.Helper()
	s.Git(s.Root, "init", "-q", dir)
	s.Git(dir, "commit", "-q", "--allow-empty", "-m", "initial commit")
}

// InitBare creates a bare repository at dir with one commit on main.
func (s *Sandbox) InitBare(dir string) {
	s.t.Helper()
	src := s.Path(".bare-source-" + filepath.Base(dir))
	s.InitRepo(src)
	s.Git(s.Root, "clone", "-q", "--bare", src, dir)
}

// InitDotBare creates, at dir, the .bare layout: a bare repository with one
// commit on main at <dir>/.bare, and a <dir>/.git file that points to it
// with the relative path git accepts, "gitdir: ./.bare".
func (s *Sandbox) InitDotBare(dir string) {
	s.t.Helper()
	src := s.Path(".dotbare-source-" + filepath.Base(dir))
	s.InitRepo(src)
	s.Git(s.Root, "clone", "-q", "--bare", src, filepath.Join(dir, ".bare"))
	s.WriteFile(filepath.Join(dir, ".git"), "gitdir: ./.bare\n")
}

// InitSeparateGitDir creates a repository with one commit on main whose
// working tree is dir and whose repository directory is gitdir, as
// --separate-git-dir does: dir/.git is a file that points to gitdir.
func (s *Sandbox) InitSeparateGitDir(dir, gitdir string) {
	s.t.Helper()
	if err := os.MkdirAll(filepath.Dir(gitdir), 0o755); err != nil {
		s.t.Fatal(err)
	}
	s.Git(s.Root, "init", "-q", "--separate-git-dir", gitdir, dir)
	s.Git(dir, "commit", "-q", "--allow-empty", "-m", "initial commit")
}

// Symlink creates a symbolic link at link that points to target. On Windows
// creating one needs a privilege the CI runner may not have; tests that use
// it run on macOS and Linux.
func (s *Sandbox) Symlink(target, link string) {
	s.t.Helper()
	if err := os.Symlink(target, link); err != nil {
		s.t.Fatal(err)
	}
}

// Junction creates a directory junction at link that points to the
// directory target, with mklink /J as a Windows user would. Junctions exist
// only on Windows.
func (s *Sandbox) Junction(target, link string) {
	s.t.Helper()
	if runtime.GOOS != "windows" {
		s.t.Fatal("directory junctions exist only on Windows")
	}
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		s.t.Fatalf("mklink /J %s %s: %v\n%s", link, target, err, out)
	}
}

// InitRemote creates a bare repository at dir with one commit on main, to
// serve as origin for Clone.
func (s *Sandbox) InitRemote(dir string) {
	s.t.Helper()
	s.InitBare(dir)
}

// Clone clones remote into dir. The clone has origin and origin/HEAD.
func (s *Sandbox) Clone(remote, dir string) {
	s.t.Helper()
	s.Git(s.Root, "clone", "-q", remote, dir)
}

// Commit adds an empty commit in dir and returns its full id.
func (s *Sandbox) Commit(dir, msg string) string {
	s.t.Helper()
	s.Git(dir, "commit", "-q", "--allow-empty", "-m", msg)
	return s.Git(dir, "rev-parse", "HEAD")
}

// Push pushes from dir with args, as in Push(clone, "origin", "HEAD:main").
func (s *Sandbox) Push(dir string, args ...string) {
	s.t.Helper()
	s.Git(dir, append([]string{"push", "-q"}, args...)...)
}

// PushFromAnotherClone advances remote as someone else would: it clones it
// into a fresh directory, commits there, and pushes HEAD to branch. It
// returns the id of the commit pushed.
func (s *Sandbox) PushFromAnotherClone(remote, branch string) string {
	s.t.Helper()
	other, err := os.MkdirTemp(s.Root, "other-clone-")
	if err != nil {
		s.t.Fatal(err)
	}
	s.Clone(remote, other)
	id := s.Commit(other, "pushed by someone else")
	s.Push(other, "origin", "HEAD:refs/heads/"+branch)
	return id
}

// BreakRemote points the remote name of repo to a directory that does not
// exist, so fetching from it fails without touching the network.
func (s *Sandbox) BreakRemote(repo, name string) {
	s.t.Helper()
	s.Git(repo, "remote", "set-url", name, s.Path("unreachable-remote"))
}

// AddWorktree adds a linked worktree at dir on a new branch.
func (s *Sandbox) AddWorktree(repo, dir, branch string) {
	s.t.Helper()
	s.Git(repo, "worktree", "add", "-q", "-b", branch, dir)
}

// AddDetachedWorktree adds a linked worktree at dir with a detached HEAD.
func (s *Sandbox) AddDetachedWorktree(repo, dir string) {
	s.t.Helper()
	s.Git(repo, "worktree", "add", "-q", "--detach", dir)
}

// LockWorktree locks the worktree at dir; an empty reason locks without one.
func (s *Sandbox) LockWorktree(repo, dir, reason string) {
	s.t.Helper()
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	s.Git(repo, append(args, dir)...)
}

// MakePrunable deletes a linked worktree's directory without telling git, so
// git reports it as prunable.
func (s *Sandbox) MakePrunable(dir string) {
	s.t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		s.t.Fatal(err)
	}
}

// Comparable returns p in a form that can be compared with paths reported by
// git or wt: absolute, with native separators and symbolic links resolved.
// When p does not exist (a prunable worktree), its longest existing ancestor
// is resolved and the rest is appended.
//
// On macOS the temporary directory is /var/folders/..., a symbolic link to
// /private/var/folders/..., and git reports the latter.
func Comparable(t testing.TB, p string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for cur := abs; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{real}, missing...)...)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		missing = append([]string{filepath.Base(cur)}, missing...)
		cur = parent
	}
}

// SetUpstream sets upstream (a remote-tracking branch, as origin/feat) as
// the upstream of branch in repo. The upstream must exist.
func (s *Sandbox) SetUpstream(repo, branch, upstream string) {
	s.t.Helper()
	s.Git(repo, "branch", "-q", "--set-upstream-to="+upstream, branch)
}

// SquashMerge merges branch into the branch checked out in dir as one new
// commit, as a squash merge on a forge does, and returns its id. The new
// commit does not have branch's tip as an ancestor.
func (s *Sandbox) SquashMerge(dir, branch string) string {
	s.t.Helper()
	s.Git(dir, "merge", "-q", "--squash", branch)
	// --allow-empty: the branch's commits may be empty, as Commit makes them.
	return s.Commit(dir, "squash merge of "+branch)
}

// ReadOnlyDir removes the write permission of dir, so that nothing inside it
// can be created or deleted, and restores it when t ends so that the sandbox
// can be cleaned up. It skips t on Windows, where directory permissions do
// not work this way, and when running as root, who ignores them. t is the
// calling test, which may be a subtest of the sandbox's: skipping through
// the sandbox's own test from a subtest would panic.
func (s *Sandbox) ReadOnlyDir(t testing.TB, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not prevent deletion on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}
