package cli

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/shell"
)

func (a *app) shellCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Set up the wt shell function",
		Args:  unknownSubcommand,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		}),
	}
	cmd.SuggestionsMinimumDistance = 2
	cmd.AddCommand(a.shellInitCommand())
	return cmd
}

// The installation lines are part of the shell-integration contract; the
// README shows the same ones.
const shellInitLong = `Print the script that defines the wt shell function and registers its
completions.

wt is a shell function that runs git-wt, because a binary cannot change the
working directory of its shell. Add the line for your shell to its file:

  zsh   ~/.zshrc                    eval "$(git-wt shell init zsh)"
  bash  ~/.bashrc                   eval "$(git-wt shell init bash)"
  fish  ~/.config/fish/config.fish  git-wt shell init fish | source
  pwsh  $PROFILE                    Invoke-Expression (& git-wt shell init pwsh | Out-String)

Completions need compinit to have run before that line in zsh, and the
bash-completion package to be loaded before it in bash. Without them, the
function still works.

Supported: zsh 5.8, bash 3.2, fish 3.3 and PowerShell 7.4, or later. On
Windows, only PowerShell is supported, and the function hides wt.exe
(Windows Terminal): type wt.exe to open it.`

func (a *app) shellInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <shell>",
		Short: "Print the shell function and completions for a shell",
		Long:  shellInitLong,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := exactArgs("shell")(cmd, args); err != nil {
				return err
			}
			if !slices.Contains(shell.Names, args[0]) {
				return &Error{
					Code:  ExitUsage,
					Msg:   fmt.Sprintf("unsupported shell %q", args[0]),
					Hints: []string{"supported shells: " + strings.Join(shell.Names, ", ")},
				}
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return withPrefix(shell.Names, toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		// Reads neither the configuration nor git: a broken configuration
		// must not break the shell's startup.
		RunE: action(func(cmd *cobra.Command, args []string) error {
			script, err := shellScript(cmd.Root(), args[0])
			if err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(struct {
					Schema string `json:"schema"`
					Shell  string `json:"shell"`
					Script string `json:"script"`
				}{"wt.shell.init.v1", args[0], script})
			}
			_, err = io.WriteString(a.env.Stdout, script)
			return err
		}),
	}
}

// shellScript composes the function for name with the completion script
// cobra derives from the same command tree that parses the command line.
func shellScript(root *cobra.Command, name string) (string, error) {
	var buf bytes.Buffer
	var err error
	switch name {
	case "zsh":
		err = root.GenZshCompletion(&buf)
	case "bash":
		err = root.GenBashCompletionV2(&buf, true)
	case "fish":
		err = root.GenFishCompletion(&buf, true)
	case "pwsh":
		err = root.GenPowerShellCompletionWithDesc(&buf)
	}
	if err != nil {
		return "", err
	}
	return shell.Script(name, buf.String())
}

// withPrefix keeps the candidates that start with prefix.
func withPrefix(candidates []string, prefix string) []cobra.Completion {
	var out []cobra.Completion
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}
