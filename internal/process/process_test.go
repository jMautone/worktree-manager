package process

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCommand(t *testing.T) {
	for _, tc := range []struct {
		goos, cmdline, comspec string
		path                   string
		args                   []string
		raw                    string
	}{
		{"darwin", "echo a && echo b", "", "/bin/sh", []string{"/bin/sh", "-c", "echo a && echo b"}, ""},
		{"linux", `claude "fix it"`, `C:\x`, "/bin/sh", []string{"/bin/sh", "-c", `claude "fix it"`}, ""},
		{"windows", `echo "a b"`, `C:\Windows\system32\cmd.exe`,
			`C:\Windows\system32\cmd.exe`, []string{`C:\Windows\system32\cmd.exe`, "/d", "/s", "/c", `echo "a b"`},
			`C:\Windows\system32\cmd.exe /d /s /c "echo "a b""`},
		{"windows", "echo a& echo b", "",
			"cmd.exe", []string{"cmd.exe", "/d", "/s", "/c", "echo a& echo b"},
			`cmd.exe /d /s /c "echo a& echo b"`},
		{"windows", "exit /b 3", `C:\Program Files\cmd.exe`,
			`C:\Program Files\cmd.exe`, []string{`C:\Program Files\cmd.exe`, "/d", "/s", "/c", "exit /b 3"},
			`"C:\Program Files\cmd.exe" /d /s /c "exit /b 3"`},
	} {
		path, args, raw := Command(tc.goos, tc.cmdline, tc.comspec)
		if path != tc.path || !slices.Equal(args, tc.args) || raw != tc.raw {
			t.Errorf("%s %q:\n got %q %q %q\nwant %q %q %q", tc.goos, tc.cmdline, path, args, raw, tc.path, tc.args, tc.raw)
		}
	}
}

func TestLookupEnvIgnoresCase(t *testing.T) {
	env := []string{"PATH=/x", `COMSPEC=C:\cmd.exe`, "EMPTY="}
	if got := lookupEnv(env, "ComSpec"); got != `C:\cmd.exe` {
		t.Errorf("ComSpec = %q", got)
	}
	if got := lookupEnv(env, "NOPE"); got != "" {
		t.Errorf("NOPE = %q", got)
	}
}

func TestDetachFlags(t *testing.T) {
	const base = createNewProcessGroup | createNoWindow
	if flags, ok := detachFlags(0, nil); !ok || flags != base|createBreakawayFromJob {
		t.Errorf("first attempt: %#x, %v; want breakaway", flags, ok)
	}
	denied := &fs.PathError{Op: "fork/exec", Path: "git-wt.exe", Err: errorAccessDenied}
	if flags, ok := detachFlags(1, denied); !ok || flags != base {
		t.Errorf("after ERROR_ACCESS_DENIED: %#x, %v; want no breakaway", flags, ok)
	}
	if _, ok := detachFlags(1, errors.New("other")); ok {
		t.Error("another attempt after an error that is not ERROR_ACCESS_DENIED")
	}
	if _, ok := detachFlags(2, denied); ok {
		t.Error("a third attempt")
	}
}

func TestStartDetachedMissingExecutable(t *testing.T) {
	out, err := os.Create(filepath.Join(t.TempDir(), "log"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	missing := filepath.Join(t.TempDir(), "missing")
	if err := StartDetached(missing, []string{missing}, t.TempDir(), os.Environ(), out); err == nil {
		t.Error("StartDetached of a missing executable: no error")
	}
}
