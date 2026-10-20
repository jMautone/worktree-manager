package cli_test

import (
	"runtime"
	"slices"
	"testing"
)

// Scenarios of Windows of batch-execution, in process: "Interpreter
// operators in a single word (Windows)" and "Line shown by a dry run
// (Windows)". They run only there; process.Line is also tested with goos
// windows on every OS.
func TestEachOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	h := newHarness(t)
	h.cwd = h.repo()

	r := h.run("each", "echo a& echo b")
	r.mustCode(t, 0)
	if got, want := outLines(r.stdout), []string{header(t, "^", h.cwd), "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("operators: stdout = %q, want %q", got, want)
	}

	r = h.run("each", "--dry-run", "git", "commit", "-m", "fix bug")
	r.mustCode(t, 0)
	if want := wouldRun(t, `git commit -m "fix bug"`, h.cwd) + "\n"; r.stdout != want {
		t.Errorf("dry run: stdout = %q, want %q", r.stdout, want)
	}
}
