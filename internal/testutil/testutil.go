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
