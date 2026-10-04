package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/term"
)

// Env is everything wt takes from the process. main builds the real one;
// tests build their own, which is what lets them run in process with a
// controlled environment and a fixed GOOS.
type Env struct {
	Args           []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	Environ        []string // environment for child processes (git); Run removes the shell protocol variables
	Getwd          func() (string, error)
	// Chdir changes the process's working directory; nil does nothing. Tests
	// that run in process leave it nil: the working directory belongs to the
	// whole test process.
	Chdir func(dir string) error
	GOOS  string // runtime.GOOS in production
	// IsTTY reports whether stdout is a terminal that renders ANSI sequences
	// (on Windows, one where virtual terminal processing could be enabled).
	IsTTY   bool
	Version string
}

// app holds the state of one run: the environment and the global flags.
type app struct {
	env Env
	git git.Runner

	json    bool
	dryRun  bool
	dir     string // -C
	noColor bool
	version bool // --version
}

// gitRunner builds the git.Runner of a run from the environment for git;
// tests wrap it to see which git commands run.
var gitRunner = func(environ []string) git.Runner { return git.Exec{Env: environ} }

// Run executes the command line in env.Args and returns the exit code. It is
// the only place that prints errors and chooses the exit code.
func Run(ctx context.Context, env Env) int {
	a := &app{env: env, git: gitRunner(shell.ChildEnviron(env.Environ, env.GOOS))}
	root := a.rootCommand()
	root.SetArgs(env.Args)
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	if len(env.Args) > 0 && (env.Args[0] == cobra.ShellCompRequestCmd || env.Args[0] == cobra.ShellCompNoDescRequestCmd) {
		// A completion request never writes to the terminal. The completion
		// scripts ignore stderr, but in PowerShell the request goes through
		// the wt function, whose stderr reaches the terminal: cobra's
		// "Completion ended with directive" line would print on every TAB.
		root.SetErr(io.Discard)
	}

	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	var status *exitStatus
	if errors.As(err, &status) {
		return status.code
	}
	var e *Error
	if !errors.As(err, &e) {
		// Commands only return *Error (see action), so anything else comes
		// from cobra's own parsing and validation.
		e = &Error{Code: ExitUsage, Msg: err.Error()}
	}
	a.printError(e, a.json || jsonRequested(env.Args))
	return e.Code
}

// jsonRequested reports whether --json appears in args, for errors raised
// before the flag could be parsed (an unknown flag earlier in the line).
func jsonRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}
	return false
}

const rootLong = `wt manages git worktrees from the terminal.

The binary is git-wt, so git also runs it as "git wt". git intercepts
"git wt --help" to look for a manual page; use "git wt -h" instead.`

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:               "wt",
		Short:             "Manage git worktrees from the terminal",
		Long:              rootLong,
		Args:              unknownSubcommand,
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: action(func(cmd *cobra.Command, args []string) error {
			if a.version {
				return a.printVersion()
			}
			return cmd.Help()
		}),
	}
	root.SuggestionsMinimumDistance = 2
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(cmd, err.Error())
	})
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.LocalFlags.FlagUsages", "{{flagUsages .LocalFlags",
		"{{.InheritedFlags.FlagUsages", "{{flagUsages .InheritedFlags",
	).Replace(root.UsageTemplate()))

	pf := root.PersistentFlags()
	pf.BoolVar(&a.json, "json", false, "print the result as JSON")
	pf.BoolVar(&a.dryRun, "dry-run", false, "show what would change without changing anything")
	// -C has no long form, like git's; see flagUsages for how it is shown.
	pf.StringVarP(&a.dir, "", "C", "", "run as if wt was started in `dir`")
	pf.BoolVar(&a.noColor, "no-color", false, "disable color")
	root.Flags().BoolVar(&a.version, "version", false, "print the version")

	root.SetHelpCommand(a.helpCommand())
	root.AddCommand(a.listCommand(), a.cdCommand(), a.createCommand(), a.removeCommand(), a.lockCommand(), a.unlockCommand(), a.pruneCommand(), a.configCommand(), a.shellCommand(), a.versionCommand())
	return root
}

func init() {
	cobra.AddTemplateFunc("flagUsages", flagUsages)
}

// shortOnly matches a flag with only a short form as pflag renders it: -C
// is registered with an empty long name ("-C, -- dir") and -x with one no
// one can type ("-x, ---x cmd").
var shortOnly = regexp.MustCompile(`-([A-Za-z]), --(?:-[A-Za-z])? `)

// flagUsages renders a flag set for help. pflag has no flags without a long
// name; the replacement drops the long form and keeps the width so the
// descriptions stay aligned.
func flagUsages(fs interface{ FlagUsages() string }) string {
	usages := fs.FlagUsages()
	var b strings.Builder
	for _, line := range strings.SplitAfter(usages, "\n") {
		if m := shortOnly.FindStringSubmatchIndex(line); m != nil {
			start, end := m[0], m[1]
			short := line[m[2]:m[3]]
			rest := line[end:]
			value, after, _ := strings.Cut(rest, " ")
			line = line[:start] + "-" + short + " " + value + strings.Repeat(" ", end-start-3) + " " + after
		}
		b.WriteString(line)
	}
	return b.String()
}

func (a *app) helpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Args:  cobra.ArbitraryArgs,
		// cobra's own help command exits 0 on an unknown topic.
		RunE: action(func(cmd *cobra.Command, args []string) error {
			target, rest, err := cmd.Root().Find(args)
			if err != nil || len(rest) > 0 {
				return &Error{
					Code:  ExitUsage,
					Msg:   fmt.Sprintf("unknown help topic %q", strings.Join(args, " ")),
					Hints: []string{"run 'wt -h' for a list of commands"},
				}
			}
			target.InitDefaultHelpFlag()
			return target.Help()
		}),
	}
}

// action adapts a command implementation so that every error it returns is
// an *Error, or the *exitStatus of a -x command; anything else is an
// execution error (exit 1).
func action(f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := f(cmd, args)
		var e *Error
		var status *exitStatus
		if err != nil && !errors.As(err, &e) && !errors.As(err, &status) {
			return &Error{Code: ExitError, Msg: err.Error()}
		}
		return err
	}
}

func usageError(cmd *cobra.Command, msg string) *Error {
	return &Error{
		Code:  ExitUsage,
		Msg:   msg,
		Hints: []string{fmt.Sprintf("run '%s -h' for usage", cmd.CommandPath())},
	}
}

// unknownSubcommand validates the arguments of a command that only groups
// subcommands: any argument is an unknown command. cobra would print help and
// exit 0 for `wt config bogus`.
func unknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q", args[0])
	if cmd.HasParent() {
		msg += fmt.Sprintf(" for %q", cmd.CommandPath())
	}
	var hints []string
	for _, s := range cmd.SuggestionsFor(args[0]) {
		hints = append(hints, fmt.Sprintf("did you mean %q?", s))
	}
	hints = append(hints, fmt.Sprintf("run '%s -h' for a list of commands", cmd.CommandPath()))
	return &Error{Code: ExitUsage, Msg: msg, Hints: hints}
}

// noArgs rejects positional arguments.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError(cmd, fmt.Sprintf("unexpected argument %q for %q", args[0], cmd.CommandPath()))
	}
	return nil
}

// exactArgs requires the named positional arguments.
func exactArgs(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < len(names) {
			return usageError(cmd, fmt.Sprintf("missing argument <%s> for %q", names[len(args)], cmd.CommandPath()))
		}
		if len(args) > len(names) {
			return usageError(cmd, fmt.Sprintf("unexpected argument %q for %q", args[len(names)], cmd.CommandPath()))
		}
		return nil
	}
}

// printError writes e to stderr: `wt: <message>` plus `hint:` lines, or one
// wt.error.v1 JSON document.
func (a *app) printError(e *Error, asJSON bool) {
	msg := oneLine(e.Msg)
	if asJSON {
		doc := struct {
			Schema  string   `json:"schema"`
			Code    int      `json:"code"`
			Message string   `json:"message"`
			Hints   []string `json:"hints,omitempty"`
		}{"wt.error.v1", e.Code, msg, e.Hints}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(doc)
		_, _ = a.env.Stderr.Write(buf.Bytes())
		return
	}
	fmt.Fprintf(a.env.Stderr, "wt: %s\n", msg)
	for _, h := range e.Hints {
		fmt.Fprintf(a.env.Stderr, "hint: %s\n", h)
	}
}

// oneLine joins a multi-line message (git's stderr can span several lines)
// so that the error stays one `wt:` line.
func oneLine(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, "; ")
}

// warn writes a warning to stderr. Warnings never change the exit code and
// are not written with --json.
func (a *app) warn(msg string) {
	if !a.json {
		fmt.Fprintf(a.env.Stderr, "wt: warning: %s\n", msg)
	}
}

// color reports whether this run's output may carry ANSI color. JSON output
// never goes through it.
func (a *app) color() bool {
	return term.UseColor(a.env.IsTTY, a.env.Getenv("NO_COLOR"), a.noColor)
}

// writeJSON writes v to stdout as the command's one JSON document.
func (a *app) writeJSON(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := a.env.Stdout.Write(buf.Bytes())
	return err
}
