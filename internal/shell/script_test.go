package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// completionStub stands in for cobra's completion script in each shell's
// syntax.
var completionStub = map[string]string{
	"zsh":  "compdef _wt wt\n_wt() { :; }\n",
	"bash": "__start_wt() { :; }\ncomplete -F __start_wt wt\n",
	"fish": "complete -c wt -e\n",
	"pwsh": "Register-ArgumentCompleter -CommandName 'wt' -ScriptBlock { }\n",
}

func TestScript(t *testing.T) {
	for _, tc := range []struct {
		shell    string
		function string
		guard    string
	}{
		{"zsh", "wt() {", "if (( $+functions[compdef] )); then\n" + completionStub["zsh"] + "fi\n"},
		{"bash", "wt() {", "if declare -F _get_comp_words_by_ref >/dev/null 2>&1; then\n" + completionStub["bash"] + "fi\n"},
		{"fish", "function wt ", completionStub["fish"]},
		{"pwsh", "function global:wt {", completionStub["pwsh"]},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			s, err := Script(tc.shell, completionStub[tc.shell])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(s, tc.function) {
				t.Errorf("script does not define the function (%q):\n%s", tc.function, s)
			}
			if !strings.Contains(s, "git-wt") || !strings.Contains(s, DirectiveVar) || !strings.Contains(s, PreviousVar) {
				t.Errorf("script does not run git-wt with the protocol variables:\n%s", s)
			}
			if !strings.HasSuffix(s, tc.guard) {
				t.Errorf("script does not end with the completion %q:\n%s", tc.guard, s)
			}
			if strings.Index(s, tc.function) > strings.Index(s, completionStub[tc.shell]) {
				t.Error("the completion comes before the function")
			}
			if strings.Contains(s, "\r") {
				t.Error("script contains CR")
			}
		})
	}
}

func TestScriptCompletionWithoutTrailingNewline(t *testing.T) {
	s, err := Script("zsh", "compdef _wt wt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(s, "compdef _wt wt\nfi\n") {
		t.Errorf("script ends with %q", s[max(0, len(s)-40):])
	}
}

func TestScriptUnknownShell(t *testing.T) {
	for _, name := range []string{"tcsh", "", "ZSH", "powershell", "ps1"} {
		_, err := Script(name, "")
		if err == nil {
			t.Errorf("Script(%q) succeeded", name)
			continue
		}
		for _, want := range Names {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Script(%q): error %q does not list %s", name, err, want)
			}
		}
	}
}

// syntaxCheck is the command line that parses the script at path without
// running it.
func syntaxCheck(shell, path string) []string {
	switch shell {
	case "zsh":
		return []string{"zsh", "-n", path}
	case "bash":
		return []string{"bash", "-n", path}
	case "fish":
		return []string{"fish", "--no-config", "-n", path}
	}
	// Arguments after -Command are part of the command, not $args.
	return []string{"pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		"$e = $null; [void][Management.Automation.Language.Parser]::ParseFile('" + path + "', [ref]$null, [ref]$e); if ($e) { $e; exit 1 }"}
}

func TestScriptSyntax(t *testing.T) {
	for _, name := range Names {
		t.Run(name, func(t *testing.T) {
			// The bash on a Windows PATH may be the WSL launcher.
			if runtime.GOOS == "windows" && name != "pwsh" {
				t.Skipf("%s is not supported on Windows", name)
			}
			if _, err := exec.LookPath(name); err != nil {
				t.Skipf("%s is not installed", name)
			}
			s, err := Script(name, completionStub[name])
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "wt."+name)
			if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
			argv := syntaxCheck(name, path)
			out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
			if err != nil || len(out) > 0 {
				t.Errorf("%s: %v\n%s", strings.Join(argv, " "), err, out)
			}
		})
	}
}
