package cli

// Exit codes. They are part of the public contract: a command never uses a
// code for a meaning other than the one documented here, and codes are never
// renumbered. Codes a command does not use yet are declared anyway.
const (
	ExitOK              = 0 // success
	ExitError           = 1 // execution error, including invalid configuration and git failures
	ExitUsage           = 2 // unknown command or flag, bad argument, invalid flag value
	ExitNotFound        = 3 // repository, worktree or branch not found
	ExitAmbiguous       = 4 // a name matches more than one target
	ExitBlocked         = 5 // blocked by state: dirty worktree, lock
	ExitConflict        = 6 // git conflict
	ExitHookFailed      = 7 // a hook failed
	ExitHookNotApproved = 8 // a repository hook has not been approved
)

// Error is a failure with its exit code and optional hints for the user. Every
// error a command returns is an *Error or becomes one with ExitError; Run is
// the only place that prints errors and picks the exit code.
type Error struct {
	Code  int
	Msg   string
	Hints []string
}

func (e *Error) Error() string { return e.Msg }
