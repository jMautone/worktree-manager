package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/approval"
	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/hook"
	"github.com/jMautone/worktree-manager/internal/process"
	"github.com/jMautone/worktree-manager/internal/shell"
)

// runHook runs one command of a hook; tests replace it.
var runHook = process.Run

// startBackground starts the process that runs the commands of the
// background event r and writes its log, and does not wait for it: a copy
// of wt running __run-hooks, detached from the terminal, with the log,
// opened here, as its output. env is the commands' environment. Tests
// replace it to run the same loop in process.
var startBackground = func(r hook.Run, env []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := openLog(r.Log)
	if err != nil {
		return err
	}
	defer log.Close()
	args := append([]string{self, runHooksName, "--event", string(r.Event), "--dir", r.Dir, "--"}, r.Commands...)
	return process.StartDetached(self, args, r.Dir, env, log)
}

// runHooksName is the hidden command the background process runs.
const runHooksName = "__run-hooks"

// runHooksCommand is what the background process of a hook event runs:
// the commands, in order, as runLogged does, writing to its own stdout,
// which is the event's log. It reads no configuration and no repository,
// and its arguments are an internal contract, not part of the CLI.
func (a *app) runHooksCommand() *cobra.Command {
	var event, dir string
	cmd := &cobra.Command{
		Use:    runHooksName + " --event <event> --dir <dir> [--] <cmd>...",
		Short:  "Run the commands of a background hook (internal)",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			e := hook.Event(event)
			if !slices.Contains(hook.Events, e) || dir == "" {
				return usageError(cmd, fmt.Sprintf("%s needs --event <event> and --dir <dir>", runHooksName))
			}
			// The exit code is no one's: nothing waits for this process.
			runLogged(hook.Run{Event: e, Dir: dir, Commands: args}, shell.ChildEnviron(a.env.Environ, a.env.GOOS), a.env.Stdout)
			return nil
		}),
	}
	// The commands are arguments, whatever they look like.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&event, "event", "", "the event the commands are of")
	cmd.Flags().StringVar(&dir, "dir", "", "the working directory of the commands")
	return cmd
}

// runLogged runs the commands of r one at a time, with no input, writing
// to out, before each, its header, then what it writes, then how it ended;
// it stops after the first that fails. It is the loop of a background
// event, and of its log.
func runLogged(r hook.Run, env []string, out io.Writer) {
	for _, c := range r.Commands {
		fmt.Fprintln(out, hook.Header(r.Event, c))
		code, err := runHook(r.Dir, c, env, nil, out, out)
		fmt.Fprintln(out, hook.Footer(code, err))
		if err != nil || code != 0 {
			return
		}
	}
}

// openLog creates the log at path, with its missing directories, and
// empties it: each run of an event replaces its log.
func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// hookPlanner decides the events of one command: their commands from cfg,
// the user's and then the repository's, rendered, and, when something will
// write one, their log. The repository's commands must be approved first.
type hookPlanner struct {
	a       *app
	ctx     context.Context
	cfg     *config.Config
	noHooks bool
	gitDir  string // where git reports the common git directory from
	common  string // the common git directory, read once and only for a log

	// Read once, and only when an event has commands of the repository.
	ids       *repoHooksID
	approvals *approval.Set
}

// repoHooksID is what an approval of the repository's hooks of a
// configuration is about: the repository whose .wt.toml declares them.
type repoHooksID struct {
	file     string // the .wt.toml
	repoPath string // that repository's main worktree, the approval's repo
	approve  string // the command that approves them, for a hint
}

// configured reports whether any of events has commands that would run.
func (p *hookPlanner) configured(events ...hook.Event) bool {
	if p.noHooks {
		return false
	}
	for _, e := range events {
		if len(p.commands(e)) > 0 {
			return true
		}
	}
	return false
}

// commands are the command templates of event e: the user's, then the
// repository's.
func (p *hookPlanner) commands(e hook.Event) []string {
	user := configList(p.cfg, e.Key())
	repo := p.cfg.RepoHooks[e.Key()]
	if len(repo) == 0 {
		return user
	}
	return append(slices.Clone(user), repo...)
}

// check fails with exit 8 when a command of the repository's hooks of any
// of events is not approved. It reads nothing when none of them has one.
func (p *hookPlanner) check(events ...hook.Event) error {
	if p.noHooks {
		return nil
	}
	has := false
	for _, e := range events {
		has = has || len(p.cfg.RepoHooks[e.Key()]) > 0
	}
	if !has {
		return nil
	}
	if p.ids == nil {
		id, err := p.a.repoHooksID(p.ctx, p.cfg)
		if err != nil {
			return err
		}
		set, err := p.a.loadApprovals()
		if err != nil {
			return err
		}
		p.ids, p.approvals = &id, &set
	}
	e, c, ok := hook.Unapproved(events, p.cfg.RepoHooks, func(e hook.Event, c string) bool {
		return p.approvals.Approved(p.ids.repoPath, e, c)
	})
	if !ok {
		return nil
	}
	return &Error{
		Code: ExitHookNotApproved,
		Msg:  fmt.Sprintf("%s hook of %s is not approved: %s", e, p.ids.file, c),
		Hints: []string{
			fmt.Sprintf("'%s' approves the hooks of this repository; 'wt hook list' shows them", p.ids.approve),
			"--no-hooks runs the command without hooks",
		},
	}
}

// plan is what event e does: its commands rendered with values, run in
// dir. Its log is set for a background event, and for a blocking one under
// --json; never under --dry-run, which writes none. With --no-hooks no
// event does anything. A command of the repository not approved fails it
// with exit 8.
func (p *hookPlanner) plan(e hook.Event, values map[string]string, dir string) (hook.Run, error) {
	if p.noHooks {
		return hook.Run{Event: e, Dir: dir}, nil
	}
	if err := p.check(e); err != nil {
		return hook.Run{}, err
	}
	r, err := hook.Plan(e, p.commands(e), values, dir, "")
	if err != nil {
		return hook.Run{}, err
	}
	if len(r.Commands) == 0 || p.a.dryRun || !e.Background() && !p.a.json {
		return r, nil
	}
	if p.common == "" {
		if p.common, err = git.CommonDir(p.ctx, p.a.git, p.gitDir); err != nil {
			return hook.Run{}, gitError(err)
		}
	}
	r.Log = hook.LogPath(p.common, values["name"], e, p.a.env.GOOS)
	return r, nil
}

// repoHooksID reads which repository the repository layer of cfg belongs
// to, from the directory of its .wt.toml, and whether it is the working
// directory's. It runs git, so only for a configuration with repository
// hooks.
func (a *app) repoHooksID(ctx context.Context, cfg *config.Config) (repoHooksID, error) {
	layerDir := filepath.Dir(cfg.RepoFile)
	repo, err := a.loadRepository(ctx, layerDir)
	if err != nil {
		return repoHooksID{}, gitError(err)
	}
	main := repo.main()
	if main == nil {
		return repoHooksID{}, &Error{Code: ExitError, Msg: "cannot find the main worktree of " + layerDir}
	}
	id := repoHooksID{file: cfg.RepoFile, repoPath: main.Path, approve: "wt -C " + layerDir + " hook approve"}
	if a.dir == "" {
		if dir, err := a.workdir(); err == nil {
			if wd, err := a.loadRepository(ctx, dir); err == nil && wd.main() != nil && wd.main().Path == main.Path {
				id.approve = "wt hook approve"
			}
		}
	}
	return id, nil
}

// approvalsFile is the path of approvals.toml.
func (a *app) approvalsFile() (string, error) {
	return config.ApprovalsFile(a.env.GOOS, a.env.Getenv, a.home())
}

// loadApprovals reads approvals.toml; a missing one is no approvals.
func (a *app) loadApprovals() (approval.Set, error) {
	path, err := a.approvalsFile()
	if err != nil {
		return approval.Set{}, err
	}
	set, err := approval.Load(path)
	if err != nil {
		return approval.Set{}, &Error{Code: ExitError, Msg: err.Error()}
	}
	return set, nil
}

// configList is the value of a list key of cfg.
func configList(cfg *config.Config, key string) []string {
	v, _ := cfg.Get(key)
	list, _ := v.Value.([]string)
	return list
}

// runBlocking runs the commands of the blocking event r one at a time,
// each after its header, and waits for them. Without --json they use wt's
// stdin and write to wt's stderr, so that stdout stays wt's own; with
// --json they get no input and write to the event's log, with the footers.
//
// A Ctrl-C reaches the command that is running, as with -x: wt waits for
// it, then runs no other and fails with 130, whatever its exit code. A
// command that fails stops the event with 7.
func (a *app) runBlocking(r hook.Run) error {
	if len(r.Commands) == 0 {
		return nil
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	env := shell.ChildEnviron(a.env.Environ, a.env.GOOS)
	out, stdin := a.env.Stderr, a.env.Stdin
	if a.json {
		log, err := openLog(r.Log)
		if err != nil {
			return &Error{Code: ExitError, Msg: fmt.Sprintf("cannot write the log of the %s hook: %v", r.Event, err)}
		}
		defer log.Close()
		out, stdin = log, nil
	}
	for _, c := range r.Commands {
		fmt.Fprintln(out, hook.Header(r.Event, c))
		code, err := runHook(r.Dir, c, env, stdin, out, out)
		if a.json {
			fmt.Fprintln(out, hook.Footer(code, err))
		}
		// The interrupt first: a command that catches it and exits with 0
		// was still interrupted.
		if interrupted := pending(interrupts); interrupted || err != nil || code != 0 {
			return hookError(r, c, code, err, interrupted, a.json)
		}
	}
	return nil
}

// hookError is the error of the blocking event r once its command c ended
// with code, or could not start with err, or was interrupted. Its hints
// say what to do about it: --no-hooks for an event before the operation,
// which did nothing; the new worktree for post-create, which is there
// anyway; and, with --json, the log, which has the commands' output.
func hookError(r hook.Run, c string, code int, err error, interrupted, asJSON bool) *Error {
	e := &Error{Code: ExitHookFailed}
	switch {
	case interrupted:
		e.Code, e.Msg = ExitInterrupted, fmt.Sprintf("interrupted: %s hook did not finish", r.Event)
	case err != nil:
		e.Msg = fmt.Sprintf("%s hook failed: %s could not start: %v", r.Event, c, err)
	default:
		e.Msg = fmt.Sprintf("%s hook failed: %s exited with %d", r.Event, c, code)
	}
	if r.Event == hook.PostCreate {
		e.Hints = append(e.Hints, "the worktree was created anyway: "+r.Dir)
	} else {
		e.Hints = append(e.Hints, "--no-hooks runs the command without hooks")
	}
	if asJSON {
		e.Hints = append(e.Hints, "the output of the hook is in "+r.Log)
	}
	return e
}

// background starts the background event r, when it has commands, and
// says where its log is. A process that cannot start is a warning: the
// command's outcome does not depend on its hooks.
func (a *app) background(r hook.Run) {
	if len(r.Commands) == 0 {
		return
	}
	if err := startBackground(r, shell.ChildEnviron(a.env.Environ, a.env.GOOS)); err != nil {
		a.warn(fmt.Sprintf("cannot start the %s hook: %v", r.Event, err))
		return
	}
	if !a.json {
		fmt.Fprintln(a.env.Stderr, hook.BackgroundNotice(r))
	}
}

// previewHook writes the --dry-run lines of r to b.
func previewHook(b io.Writer, r hook.Run) {
	for _, line := range hook.PreviewLines(r) {
		fmt.Fprintln(b, line)
	}
}
