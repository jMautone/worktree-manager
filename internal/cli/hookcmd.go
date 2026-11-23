package cli

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/approval"
	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/hook"
)

func (a *app) hookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "List and approve the hooks of the current repository",
		Long: `List and approve the hooks of the current repository.

A repository may declare hooks in the [hooks] table of its .wt.toml, with
the events and the form of the user's (see "wt config --help"). They run
after the user's commands of the same event, and only once approved: until
then "wt create", "wt cd" and "wt remove" fail with exit code 8, before
doing anything, whenever an event they would fire has a command of the
repository that is not approved.

An approval is the repository (its main worktree's path), the event and
the SHA-256 of the command as written in .wt.toml. One approval covers
every worktree of the repository; changing a single character of the
command asks for it again. Approvals live in approvals.toml, next to the
user's configuration file ("wt config path" shows it). Delete a table of
that file to revoke an approval.`,
		Args: unknownSubcommand,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		}),
	}
	cmd.SuggestionsMinimumDistance = 2
	cmd.AddCommand(a.hookListCommand(), a.hookApproveCommand())
	return cmd
}

// repoHooks is what wt hook list and wt hook approve work on: the hooks of
// the working directory's repository and its repository layer.
type repoHooks struct {
	cfg       *config.Config
	repoPath  string // the main worktree's path, the approvals' repo
	approvals approval.Set
}

// loadRepoHooks reads the working directory's repository, its
// configuration and, when its .wt.toml has hooks, the approvals. Outside a
// repository it fails with exit 3.
func (a *app) loadRepoHooks(ctx context.Context) (repoHooks, error) {
	dir, err := a.workdir()
	if err != nil {
		return repoHooks{}, err
	}
	repo, err := a.loadRepository(ctx, dir)
	if err != nil {
		return repoHooks{}, gitError(err)
	}
	cfg, err := a.loadConfig(repo.root())
	if err != nil {
		return repoHooks{}, err
	}
	rh := repoHooks{cfg: cfg}
	if main := repo.main(); main != nil {
		rh.repoPath = main.Path
	}
	if len(cfg.RepoHooks) > 0 {
		if rh.approvals, err = a.loadApprovals(); err != nil {
			return repoHooks{}, err
		}
	}
	return rh, nil
}

// entries are the hook commands of rh, in the order they would run.
func (rh repoHooks) entries() []hook.Entry {
	user := map[string][]string{}
	for _, e := range hook.Events {
		user[e.Key()] = configList(rh.cfg, e.Key())
	}
	return hook.Entries(user, rh.cfg.RepoHooks, func(e hook.Event, c string) bool {
		return rh.approvals.Approved(rh.repoPath, e, c)
	})
}

type hookEntryJSON struct {
	Event    string `json:"event"`
	Source   string `json:"source"`
	Command  string `json:"command"`
	Approved bool   `json:"approved"`
}

func (a *app) hookListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the hook commands that would run in this repository",
		Long: `List the hook commands that would run in this repository.

One row per command, in the order they would run: by event, and within an
event the user's before the repository's. SOURCE is user or repo; APPROVED
is yes or no for the repository's commands, and - for the user's, which
need no approval. With no hook, it prints nothing.`,
		Args: noArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			rh, err := a.loadRepoHooks(cmd.Context())
			if err != nil {
				return err
			}
			entries := rh.entries()
			if a.json {
				hooks := make([]hookEntryJSON, 0, len(entries))
				for _, e := range entries {
					hooks = append(hooks, hookEntryJSON{string(e.Event), string(e.Source), e.Command, e.Approved})
				}
				var file *string
				if rh.cfg.RepoFile != "" {
					file = &rh.cfg.RepoFile
				}
				return a.writeJSON(struct {
					Schema   string          `json:"schema"`
					RepoPath string          `json:"repo_path"`
					RepoFile *string         `json:"repo_file"`
					Hooks    []hookEntryJSON `json:"hooks"`
				}{"wt.hook.list.v1", rh.repoPath, file, hooks})
			}
			if len(entries) == 0 {
				return nil
			}
			rows := [][]cell{{
				{text: "EVENT", style: styleBold},
				{text: "SOURCE", style: styleBold},
				{text: "APPROVED", style: styleBold},
				{text: "COMMAND", style: styleBold},
			}}
			for _, e := range entries {
				approved := "-"
				if e.Source == hook.SourceRepo {
					approved = "no"
					if e.Approved {
						approved = "yes"
					}
				}
				rows = append(rows, []cell{{text: string(e.Event)}, {text: string(e.Source)}, {text: approved}, {text: e.Command}})
			}
			var buf bytes.Buffer
			if err := renderTable(&buf, rows, a.color()); err != nil {
				return err
			}
			_, err = a.env.Stdout.Write(buf.Bytes())
			return err
		}),
	}
}

type hookApprovedJSON struct {
	Event   string `json:"event"`
	Command string `json:"command"`
}

func (a *app) hookApproveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "approve [<event>...]",
		Short: "Approve the hooks of this repository that are not approved yet",
		Long: `Approve the hooks of this repository that are not approved yet.

With no event, every command of the repository's .wt.toml that is not
approved; with events, only theirs. Each approval is printed as
"approved <event> hook: <command>", or "nothing to approve". With
--dry-run, the approvals are printed as "would approve ..." and nothing is
written.

An approved command still runs whatever it reads from the repository:
approving "npm ci" approves the scripts of its package.json, and "make"
its Makefile.`,
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if !slices.Contains(hook.Events, hook.Event(arg)) {
					return usageError(cmd, fmt.Sprintf("unknown hook event %q", arg))
				}
			}
			return nil
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			var events []cobra.Completion
			for _, e := range hook.Events {
				if !slices.Contains(args, string(e)) {
					events = append(events, string(e))
				}
			}
			return events, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: action(func(cmd *cobra.Command, args []string) error {
			rh, err := a.loadRepoHooks(cmd.Context())
			if err != nil {
				return err
			}
			approved := []hookApprovedJSON{}
			for _, e := range rh.entries() {
				if e.Source != hook.SourceRepo || e.Approved || len(args) > 0 && !slices.Contains(args, string(e.Event)) {
					continue
				}
				if rh.approvals.Add(rh.repoPath, e.Event, e.Command) {
					approved = append(approved, hookApprovedJSON{string(e.Event), e.Command})
				}
			}
			if len(approved) > 0 && !a.dryRun {
				path, err := a.approvalsFile()
				if err != nil {
					return err
				}
				if err := approval.Save(path, rh.approvals); err != nil {
					return &Error{Code: ExitError, Msg: err.Error()}
				}
			}
			if a.json {
				return a.writeJSON(struct {
					Schema   string             `json:"schema"`
					Approved []hookApprovedJSON `json:"approved"`
				}{"wt.hook.approve.v1", approved})
			}
			var buf bytes.Buffer
			verb := "approved"
			if a.dryRun {
				verb = "would approve"
			}
			for _, e := range approved {
				fmt.Fprintf(&buf, "%s %s hook: %s\n", verb, e.Event, e.Command)
			}
			if len(approved) == 0 {
				fmt.Fprintln(&buf, "nothing to approve")
			}
			_, err = a.env.Stdout.Write(buf.Bytes())
			return err
		}),
	}
}
