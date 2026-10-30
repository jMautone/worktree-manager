package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/config"
	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/hook"
	"github.com/jMautone/worktree-manager/internal/process"
	"github.com/jMautone/worktree-manager/internal/shell"
	"github.com/jMautone/worktree-manager/internal/template"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

const createLong = `Create a worktree of the current repository on a new branch, and move the
shell into it.

<repo>:<name> creates it in the repository <repo> under the roots in
repos_root instead, from anywhere, as if wt create <name> had started in
its main worktree: its branches, its remotes, its default branch and its
.wt.toml. <repo> is matched as in "wt cd <repo>:".

The branch is <name> after branch_prefix, or the value of -b as given. It
starts at --base, else at default_base, else at the repository's default
branch (what origin/HEAD points to, else the branch of the main worktree),
and has no upstream. wt create only creates new branches: if the branch
exists, locally or on a remote, nothing is created. When the base is
<remote>/<branch> and fetch_before_create is true, the remote is fetched
first.

The worktree goes where worktree_path says, by default
{repo_parent}/{repo}.worktrees/{name|sanitize}. sanitize turns any name
into a directory name that is valid on every OS: feature/x becomes
feature-x.

The shell moves into the new worktree when create_cd is true, the default,
through the wt shell function (see "wt shell init --help"). --cd and
--no-cd override create_cd for one run.

-x runs <cmd> in the new worktree, in this terminal, and wt exits with its
exit code. <cmd> runs with sh -c on macOS and Linux and with cmd.exe on
Windows, so their quoting and operators apply: && in sh, & in cmd. In
PowerShell the line goes through pwsh first and then through cmd.exe:
write it in single quotes, as in -x 'claude "fix it"'.

Hooks: wt create runs the commands of pre-create in the main worktree
before it creates anything, and those of post-create in the new worktree
once it exists, before the shell moves and before -x. It waits for both,
and their output goes to stderr. When a pre-create command fails, nothing
is created; when a post-create one fails, the worktree stays and the shell
moves into it, but -x does not run. Either way wt exits with 7.
--no-hooks runs neither; see "wt config --help" for how to set them.`

// createFlags are the flags of wt create.
type createFlags struct {
	branch  string
	base    string
	exec    string
	cd      bool
	noCD    bool
	noHooks bool
}

// exitStatus ends the command with code and prints nothing: the exit code
// of the -x command, which replaces wt's own codes.
type exitStatus struct{ code int }

func (e *exitStatus) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// runCommand runs the -x command; tests replace it to make it fail to start.
var runCommand = process.Run

func (a *app) createCommand() *cobra.Command {
	var f createFlags
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a worktree on a new branch and move the shell into it",
		Long:  createLong,
		Args:  exactArgs("name"),
		// A name is new: only the repositories of <repo>:<name> are offered.
		ValidArgsFunction: a.completeCreateName,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			if err := f.validate(cmd, args[0], a.json); err != nil {
				return err
			}
			plan, dir, err := a.planCreate(cmd, args[0], f)
			if err != nil {
				return err
			}
			if a.dryRun {
				return a.previewCreate(plan)
			}
			return a.create(cmd.Context(), plan, dir)
		}),
	}
	fs := cmd.Flags()
	fs.StringVarP(&f.branch, "branch", "b", "", "create `branch`, as given, instead of branch_prefix and <name>")
	fs.StringVar(&f.base, "base", "", "start the branch at `ref` instead of the default base")
	// -x has no long form. Its internal name starts with -, which pflag
	// never parses as a long flag (---x is a syntax error), so that it
	// does not collide with -C, which has the empty name.
	fs.StringVarP(&f.exec, "-x", "x", "", "run `cmd` in the new worktree and exit with its exit code")
	fs.BoolVar(&f.cd, "cd", false, "move the shell into the new worktree, whatever create_cd says")
	fs.BoolVar(&f.noCD, "no-cd", false, "do not move the shell")
	fs.BoolVar(&f.noHooks, "no-hooks", false, "run no hook")
	_ = cmd.RegisterFlagCompletionFunc("base", a.completeBases)
	_ = cmd.RegisterFlagCompletionFunc("branch", cobra.NoFileCompletions)
	_ = cmd.RegisterFlagCompletionFunc("-x", cobra.NoFileCompletions)
	return cmd
}

// validate rejects the usage errors that need neither the repository nor
// the configuration.
func (f createFlags) validate(cmd *cobra.Command, name string, asJSON bool) error {
	if repo, rest, qualified := workspace.SplitQualified(name); qualified {
		if repo == "" {
			return qualifiedUsage(name, repo, rest)
		}
		name = rest
	}
	if err := worktree.ValidateName(name); err != nil {
		return &Error{Code: ExitUsage, Msg: err.Error()}
	}
	switch {
	case f.cd && f.noCD:
		return usageError(cmd, "--cd and --no-cd cannot be used together")
	case cmd.Flags().Changed("-x") && strings.TrimSpace(f.exec) == "":
		return usageError(cmd, "-x needs a command")
	case f.exec != "" && asJSON:
		return usageError(cmd, "-x cannot be used with --json")
	}
	return nil
}

// planCreate reads everything wt create needs and decides what it would
// do. It only reads, except for the fetch (not under --dry-run); every
// check that can fail happens here, before anything is created. It also
// returns the directory git runs in for the target repository.
//
// <repo>:<name> creates the worktree in the repository <repo> names, as if
// wt create <name> had started there: from here on, dir and repo are that
// repository's, and its main worktree's .wt.toml is the repository layer.
func (a *app) planCreate(cmd *cobra.Command, name string, f createFlags) (worktree.CreatePlan, string, error) {
	ctx := cmd.Context()
	goos := a.env.GOOS
	dir, err := a.workdir()
	if err != nil {
		return worktree.CreatePlan{}, "", err
	}
	repoSide, nameSide, qualified := workspace.SplitQualified(name)
	repo, err := a.loadRepository(ctx, dir)
	var notRepo *git.NotRepoError
	switch {
	case err != nil && !errors.As(err, &notRepo):
		return worktree.CreatePlan{}, "", gitError(err)
	case notRepo != nil && !qualified:
		e := gitError(notRepo).(*Error)
		if base, err := a.loadConfigQuiet(""); err == nil && len(reposRoot(base)) > 0 {
			e.Hints = append(e.Hints, fmt.Sprintf("run 'wt create <repo>:%s' to create it in a repository under repos_root", name))
		}
		return worktree.CreatePlan{}, "", e
	}
	root := repo.root()
	if qualified {
		name = nameSide
		base, err := a.loadConfigQuiet("")
		if err != nil {
			return worktree.CreatePlan{}, "", err
		}
		r, names, err := a.matchRepo(base, repoSide)
		if err != nil {
			return worktree.CreatePlan{}, "", err
		}
		target, gitDir, other, err := a.readRepo(ctx, dir, repo, names.repos, r)
		if err != nil {
			return worktree.CreatePlan{}, "", err
		}
		if other {
			root = otherRoot(target)
		}
		repo, dir = target, gitDir
	}
	cfg, err := a.loadConfig(root)
	if err != nil {
		return worktree.CreatePlan{}, "", err
	}
	var main worktree.Worktree
	for _, w := range repo.worktrees {
		if w.Main {
			main = w
		}
	}

	// The branch.
	prefix := configString(cfg, "branch_prefix")
	branch := worktree.BranchFor(name, f.branch, prefix)
	explicit := cmd.Flags().Changed("branch")
	if explicit {
		branch = f.branch
	}
	if err := git.CheckBranchName(ctx, a.git, dir, branch); err != nil {
		var ibe *git.InvalidBranchError
		if !errors.As(err, &ibe) {
			return worktree.CreatePlan{}, "", gitError(err)
		}
		e := &Error{Code: ExitUsage, Msg: ibe.Error()}
		if prefix != "" && !explicit {
			e.Hints = []string{fmt.Sprintf("it is branch_prefix (%q) followed by <name>", prefix)}
		}
		return worktree.CreatePlan{}, "", e
	}

	// The path.
	tmpl, err := template.Parse(configString(cfg, "worktree_path"), template.WorktreePathVars)
	if err != nil {
		return worktree.CreatePlan{}, "", err
	}
	rendered := tmpl.Render(worktree.PathVars(main, name, branch, goos))
	path, err := worktree.ResolvePath(rendered, main.Path, a.home(), goos)
	if err != nil {
		return worktree.CreatePlan{}, "", &Error{Code: ExitUsage, Msg: err.Error()}
	}
	if err := worktree.CheckWindowsPath(path, goos); err != nil {
		return worktree.CreatePlan{}, "", &Error{
			Code:  ExitUsage,
			Msg:   err.Error(),
			Hints: []string{"apply the sanitize filter in worktree_path, as in {name|sanitize}"},
		}
	}
	if _, err := os.Lstat(path); err == nil {
		return worktree.CreatePlan{}, "", &Error{Code: ExitBlocked, Msg: "path already exists: " + path}
	}
	if _, ok := worktree.RegisteredAt(repo.worktrees, path, goos); ok {
		return worktree.CreatePlan{}, "", &Error{
			Code:  ExitBlocked,
			Msg:   fmt.Sprintf("a worktree is registered at %s, but its directory is missing", path),
			Hints: []string{"run 'git worktree prune' to forget it"},
		}
	}

	// A local branch is checked before touching the network.
	refs, err := git.Refs(ctx, a.git, dir, "refs/heads/"+branch)
	if err != nil {
		return worktree.CreatePlan{}, "", gitError(err)
	}
	if err := worktree.BranchTaken(branch, refs, nil, repo.worktrees); err != nil {
		return worktree.CreatePlan{}, "", branchExistsError(err)
	}

	// The base, fetched first when it is on a remote.
	base := f.base
	if base == "" {
		base = configString(cfg, "default_base")
	}
	if base == "" {
		if base, err = a.defaultBranch(ctx, dir, main); err != nil {
			return worktree.CreatePlan{}, "", err
		}
	}
	remotes, err := git.Remotes(ctx, a.git, dir)
	if err != nil {
		return worktree.CreatePlan{}, "", gitError(err)
	}
	var fetch string
	if configBool(cfg, "fetch_before_create") {
		fetch, _ = worktree.FetchRemote(base, remotes)
	}
	if fetch != "" && !a.dryRun {
		if err := git.Fetch(ctx, a.git, dir, fetch); err != nil {
			a.warn(fmt.Sprintf("cannot fetch %s (%s); using the references already here", fetch, firstGitLine(err)))
		}
	}
	head, err := git.ResolveCommit(ctx, a.git, dir, base)
	if errors.Is(err, git.ErrNoRevision) {
		return worktree.CreatePlan{}, "", &Error{Code: ExitNotFound, Msg: "base not found: " + base}
	}
	if err != nil {
		return worktree.CreatePlan{}, "", gitError(err)
	}

	// Remote-tracking branches, after the fetch, to see what was just pushed.
	if len(remotes) > 0 {
		patterns := make([]string, len(remotes))
		for i, r := range remotes {
			patterns[i] = "refs/remotes/" + r + "/" + branch
		}
		if refs, err = git.Refs(ctx, a.git, dir, patterns...); err != nil {
			return worktree.CreatePlan{}, "", gitError(err)
		}
		if err := worktree.BranchTaken(branch, refs, remotes, repo.worktrees); err != nil {
			return worktree.CreatePlan{}, "", branchExistsError(err)
		}
	}

	plan := worktree.CreatePlan{
		Name:   name,
		Path:   path,
		Branch: branch,
		Base:   base,
		Head:   head,
		Fetch:  fetch,
		CD:     f.cd || configBool(cfg, "create_cd") && !f.noCD,
		Exec:   f.exec,
	}

	// The hooks, with the new worktree's values: its NAME is the last
	// component of its path, as wt list will show it.
	hooks := &hookPlanner{a: a, ctx: ctx, cfg: cfg, noHooks: f.noHooks, gitDir: dir}
	values := worktree.HookVars(main, worktree.Worktree{Path: path, Branch: branch}, base, goos)
	if plan.PreHook, err = hooks.plan(hook.PreCreate, values, main.Path); err != nil {
		return worktree.CreatePlan{}, "", err
	}
	if plan.PostHook, err = hooks.plan(hook.PostCreate, values, path); err != nil {
		return worktree.CreatePlan{}, "", err
	}
	return plan, dir, nil
}

// readDefaultBranch reads what DefaultBranch decides with: origin/HEAD and,
// for a bare repository, which git does not report the branch of, its HEAD.
func (a *app) readDefaultBranch(ctx context.Context, dir string, main worktree.Worktree) (string, bool, error) {
	originHEAD, err := git.OriginHEAD(ctx, a.git, dir)
	if err != nil {
		return "", false, gitError(err)
	}
	var bareHEAD string
	if originHEAD == "" && main.Bare {
		if bareHEAD, err = git.SymbolicHEAD(ctx, a.git, main.Path); err != nil {
			return "", false, gitError(err)
		}
	}
	branch, ok := worktree.DefaultBranch(originHEAD, main, bareHEAD)
	return branch, ok, nil
}

// defaultBranch is the repository's default branch, which wt create needs.
func (a *app) defaultBranch(ctx context.Context, dir string, main worktree.Worktree) (string, error) {
	branch, ok, err := a.readDefaultBranch(ctx, dir, main)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &Error{
			Code:  ExitNotFound,
			Msg:   "the repository has no default branch",
			Hints: []string{"pass --base <ref>", "or set default_base in the configuration"},
		}
	}
	return branch, nil
}

func branchExistsError(err error) error {
	var be *worktree.BranchExistsError
	if !errors.As(err, &be) {
		return err
	}
	e := &Error{Code: ExitBlocked, Msg: be.Error()}
	switch {
	case be.Remote != "":
		e.Hints = []string{"choose another branch with -b"}
	case be.Worktree != nil:
		e.Hints = []string{fmt.Sprintf("it is checked out in worktree %s: run 'wt cd %s'", be.Worktree.Name, be.Worktree.Name)}
	}
	return e
}

// firstGitLine is the first line of git's message in err.
func firstGitLine(err error) string {
	msg := err.Error()
	var ce *git.CommandError
	if errors.As(err, &ce) && ce.Stderr != "" {
		msg = ce.Stderr
	}
	line, _, _ := strings.Cut(msg, "\n")
	return strings.TrimSpace(line)
}

// previewCreate prints what wt create would do, from the same plan.
func (a *app) previewCreate(plan worktree.CreatePlan) error {
	if plan.CD && !shell.Active(a.env.Getenv) {
		a.warnShellStays()
	}
	if a.json {
		return a.writeJSON(createDocument(plan))
	}
	var b strings.Builder
	if plan.Fetch != "" {
		fmt.Fprintf(&b, "would fetch %s\n", plan.Fetch)
	}
	previewHook(&b, plan.PreHook)
	fmt.Fprintf(&b, "would create worktree %s on new branch %s from %s\n", plan.Path, plan.Branch, plan.Base)
	previewHook(&b, plan.PostHook)
	if plan.CD && shell.Active(a.env.Getenv) {
		fmt.Fprintf(&b, "would change directory to %s\n", plan.Path)
	}
	if plan.Exec != "" {
		fmt.Fprintf(&b, "would run %s\n", plan.Exec)
	}
	_, err := a.env.Stdout.Write([]byte(b.String()))
	return err
}

// create carries out the plan, with git running in dir: pre-create, the
// branch, the worktree, post-create, the shell, and then the output, so
// that stdout stays empty when a hook fails; -x last.
func (a *app) create(ctx context.Context, plan worktree.CreatePlan, dir string) error {
	if err := a.runBlocking(plan.PreHook); err != nil {
		return err
	}

	// Two steps, not worktree add -b: git branch fails when the branch
	// exists, so a branch it created is wt's to delete if the worktree
	// cannot be added. The branch starts at the resolved commit, which is
	// what --dry-run and --json report.
	if err := git.CreateBranch(ctx, a.git, dir, plan.Branch, plan.Head); err != nil {
		return gitError(err)
	}
	if err := git.AddWorktree(ctx, a.git, dir, plan.Path, plan.Branch); err != nil {
		if derr := git.DeleteBranch(ctx, a.git, dir, plan.Branch); derr != nil {
			a.warn(fmt.Sprintf("cannot delete the branch %s: %s", plan.Branch, firstGitLine(derr)))
		}
		return gitError(err)
	}

	if err := a.runBlocking(plan.PostHook); err != nil {
		// The worktree is there, ready or not: the shell moves into it as
		// it would have, so the user can finish what the hook did not.
		if derr := a.moveIntoCreated(plan); derr != nil {
			a.warn(derr.Error())
		}
		return err
	}
	if err := a.moveIntoCreated(plan); err != nil {
		return err
	}

	if a.json {
		if err := a.writeJSON(createDocument(plan)); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(a.env.Stdout, "created worktree %s on new branch %s from %s\n", plan.Path, plan.Branch, plan.Base); err != nil {
		return err
	}

	if plan.Exec == "" {
		return nil
	}
	code, err := runCommand(plan.Path, plan.Exec, shell.ChildEnviron(a.env.Environ, a.env.GOOS), a.env.Stdin, a.env.Stdout, a.env.Stderr)
	if err != nil {
		return &Error{Code: ExitError, Msg: fmt.Sprintf("cannot run %s: %v", plan.Exec, err)}
	}
	if code != 0 {
		return &exitStatus{code: code}
	}
	return nil
}

// moveIntoCreated writes the new worktree as the directive, when the plan
// moves the shell and the shell integration is active.
func (a *app) moveIntoCreated(plan worktree.CreatePlan) error {
	if !plan.CD {
		return nil
	}
	if !shell.Active(a.env.Getenv) {
		a.warnShellStays()
		return nil
	}
	if err := shell.WriteDirective(a.env.Getenv(shell.DirectiveVar), plan.Path); err != nil {
		return &Error{Code: ExitError, Msg: fmt.Sprintf("cannot write the directive file: %v", err)}
	}
	return nil
}

func (a *app) warnShellStays() {
	a.warn("shell integration is not active, so the shell stays where it is; see 'wt shell init --help'")
}

// createJSON is the wt.create.v1 document.
type createJSON struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
	Head   string `json:"head"`
}

func createDocument(plan worktree.CreatePlan) createJSON {
	return createJSON{"wt.create.v1", plan.Name, plan.Path, plan.Branch, plan.Base, plan.Head}
}

// completeCreateName offers, for <name>, "<display name>:" for every
// repository under the roots whose display name starts with the word,
// ignoring case, with no space after it: the rest is a new name, which
// nothing can complete. A word with ":" is past the repository.
func (a *app) completeCreateName(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 || strings.Contains(toComplete, ":") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	base, err := a.loadConfigQuiet("")
	if err != nil || len(reposRoot(base)) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	repos := a.discoverRepos(base, func(string) {})
	var offered []cobra.Completion
	for _, name := range workspace.DisplayNames(repos, a.env.GOOS) {
		if workspace.HasPrefixFold(name, toComplete) {
			offered = append(offered, name+":")
		}
	}
	return offered, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

// completeBases offers the local and remote-tracking branches for --base,
// without <remote>/HEAD, of the repository <name> designates when it is
// <repo>:<name>, and otherwise of the working directory's. Any error
// offers nothing.
func (a *app) completeBases(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	dir, err := a.workdir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) > 0 {
		if side, _, qualified := workspace.SplitQualified(args[0]); qualified {
			base, err := a.loadConfigQuiet("")
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			repos := a.discoverRepos(base, func(string) {})
			r, err := workspace.MatchSide(repos, side, a.env.GOOS)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			dir = r.Path
		}
	}
	// Full names, shortened here: git's refname:short writes origin/HEAD
	// as origin, and a branch that is also a tag as heads/<branch>.
	refs, err := git.Refs(cmd.Context(), a.git, dir, "refs/heads", "refs/remotes")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, ref := range refs {
		name, ok := strings.CutPrefix(ref, "refs/heads/")
		if !ok {
			name = strings.TrimPrefix(ref, "refs/remotes/")
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
		}
		names = append(names, name)
	}
	return withPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func configString(cfg *config.Config, key string) string {
	v, _ := cfg.Get(key)
	s, _ := v.Value.(string)
	return s
}

func configBool(cfg *config.Config, key string) bool {
	v, _ := cfg.Get(key)
	b, _ := v.Value.(bool)
	return b
}
