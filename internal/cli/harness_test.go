package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
	"github.com/jMautone/worktree-manager/internal/testutil"
)

// harness runs cli.Run in process against a sandbox: real git, isolated
// configuration, and a working directory the test chooses.
type harness struct {
	t   *testing.T
	sb  *testutil.Sandbox
	cwd string
	tty bool
	// goos is the GOOS wt runs with; "" is the test's own.
	goos string
	// chdir is Env.Chdir; nil, as in most tests, does nothing.
	chdir func(string) error
}

type result struct {
	code           int
	stdout, stderr string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	sb := testutil.New(t)
	outside := sb.Path("outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, sb: sb, cwd: outside}
}

func (h *harness) run(args ...string) result {
	h.t.Helper()
	goos := h.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Args:    args,
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Getenv:  h.sb.Getenv,
		Environ: h.sb.Environ(),
		Getwd:   func() (string, error) { return h.cwd, nil },
		Chdir:   h.chdir,
		GOOS:    goos,
		IsTTY:   h.tty,
		Version: "1.2.3-test",
	})
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// repo creates a repository with one commit at <sandbox>/repo.
func (h *harness) repo() string {
	h.t.Helper()
	repo := h.sb.Path("repo")
	h.sb.InitRepo(repo)
	return repo
}

// userConfigPath is where the sandbox's user configuration file lives.
func (h *harness) userConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(h.sb.Getenv("APPDATA"), "wt", "config.toml")
	}
	return filepath.Join(h.sb.Getenv("XDG_CONFIG_HOME"), "wt", "config.toml")
}

func (h *harness) userConfig(content string) string {
	h.t.Helper()
	p := h.userConfigPath()
	h.sb.WriteFile(p, content)
	return p
}

func (r result) mustCode(t *testing.T, code int) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.stdout, r.stderr)
	}
}

// decodeOne decodes s as exactly one JSON document.
func decodeOne(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, s)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("more than one JSON document:\n%s", s)
	}
	return doc
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
