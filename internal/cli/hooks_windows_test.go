package cli_test

import (
	"path/filepath"
	"runtime"
	"testing"
)

// Scenario "Interpreter operators (Windows)" of hooks: cmd.exe's & runs
// both commands.
func TestCreateHookInterpreterOperatorsWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("post-create", "echo a> a.txt& echo b> b.txt"))
	path := h.mustCreate("feat", "feat")
	if !exists(filepath.Join(path, "a.txt")) || !exists(filepath.Join(path, "b.txt")) {
		t.Error("a.txt or b.txt is missing")
	}
}

// Scenario "The command waits (Windows)" of hooks: -x starts once
// post-create is done, a second later.
func TestCreateHookWaitsWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	h := newHarness(t)
	cloneRepo(h)
	h.hooks("", hookLine("post-create", "ping -n 2 127.0.0.1 > nul& echo x> prepared"))
	h.run("create", "feat", "-x", "if exist prepared (exit /b 0) else (exit /b 1)").mustCode(t, 0)
}
