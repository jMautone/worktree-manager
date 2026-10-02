package process

import (
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
