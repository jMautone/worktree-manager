package shell

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed scripts
var scripts embed.FS

// files maps each supported shell to its script in scripts/.
var files = map[string]string{
	"zsh":  "wt.zsh",
	"bash": "wt.bash",
	"fish": "wt.fish",
	"pwsh": "wt.ps1",
}

// The environment variables of the protocol between the function and the
// binary. They are not configuration keys.
const (
	// DirectiveVar names the file the binary writes the destination to.
	DirectiveVar = "WT_DIRECTIVE_CD_FILE"
	// PreviousVar holds the directory the shell was in before the function
	// last changed it, in this shell session.
	PreviousVar = "WT_PREVIOUS_DIR"
)

// Names lists the supported shells, in the order help and diagnostics show
// them.
var Names = []string{"zsh", "bash", "fish", "pwsh"}

// Active reports whether this run of the binary was started by the function:
// the directive variable is set and not empty.
func Active(getenv func(string) string) bool {
	return getenv(DirectiveVar) != ""
}

// Script returns what `wt shell init <name>` prints: the function wt for the
// shell, followed by completion, the completion script cobra generated for
// it. In zsh and bash the completion is registered only when the shell's
// completion system was loaded first; without it, the script still loads
// silently and the function works.
func Script(name, completion string) (string, error) {
	file, ok := files[name]
	if !ok {
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", name, strings.Join(Names, ", "))
	}
	fn, err := scripts.ReadFile("scripts/" + file)
	if err != nil {
		return "", err
	}
	completion = strings.TrimRight(completion, "\n") + "\n"
	var b strings.Builder
	b.Write(fn)
	b.WriteString("\n")
	switch name {
	case "zsh":
		// cobra's script starts with `compdef _wt wt`, which only exists
		// once compinit has run.
		b.WriteString("if (( $+functions[compdef] )); then\n" + completion + "fi\n")
	case "bash":
		// cobra's script calls _get_comp_words_by_ref, from the
		// bash-completion package, on every TAB.
		b.WriteString("if declare -F _get_comp_words_by_ref >/dev/null 2>&1; then\n" + completion + "fi\n")
	default:
		// fish: complete is a builtin, and cobra erases the previous
		// completions first. pwsh: Register-ArgumentCompleter replaces
		// the previous completer.
		b.WriteString(completion)
	}
	return b.String(), nil
}

// ChildEnviron returns environ without the protocol variables, for the
// processes wt starts: a program they run must not be able to write to the
// shell's directive file. Variable names are compared the way goos does:
// ignoring case on windows.
func ChildEnviron(environ []string, goos string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if !isProtocolVar(key, goos) {
			out = append(out, kv)
		}
	}
	return out
}

func isProtocolVar(key, goos string) bool {
	for _, v := range []string{DirectiveVar, PreviousVar} {
		if key == v || goos == "windows" && strings.EqualFold(key, v) {
			return true
		}
	}
	return false
}
