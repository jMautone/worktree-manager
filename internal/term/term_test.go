package term

import (
	"os"
	"runtime"
	"testing"
)

func TestUseColor(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tty         bool
		noColorEnv  string
		noColorFlag bool
		want        bool
	}{
		{"terminal", true, "", false, true},
		{"piped", false, "", false, false},
		{"NO_COLOR set", true, "1", false, false},
		{"NO_COLOR set to any value", true, "false", false, false},
		{"--no-color", true, "", true, false},
		{"piped with NO_COLOR and --no-color", false, "1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := UseColor(tc.tty, tc.noColorEnv, tc.noColorFlag); got != tc.want {
				t.Errorf("UseColor(tty=%v, NO_COLOR=%q, --no-color=%v) = %v, want %v",
					tc.tty, tc.noColorEnv, tc.noColorFlag, got, tc.want)
			}
		})
	}
}

func TestIsTerminalFalseForRegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Error("a regular file reported as a terminal")
	}
	// On unix any terminal takes ANSI sequences; on Windows a file has no
	// console mode to enable them in.
	if runtime.GOOS == "windows" && EnableANSI(f) {
		t.Error("EnableANSI succeeded on a regular file")
	}
}
