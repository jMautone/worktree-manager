// Package cli owns the command contract: parsing, dispatch, strict flag
// validation, exit codes, --dry-run, --json and help.
//
// It is the single source of truth from which help text, shell completions and
// the per-command documentation are generated. Adding a command anywhere else
// is a bug.
//
// Decision/effect boundary: this package decides what to run and returns a
// plan; it does not touch git or the filesystem.
package cli
