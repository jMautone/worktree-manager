// Package approval decides whether a command of a repository's hooks may
// run: an approval is the repository (its main worktree's path), the event
// and the SHA-256 of the command as written in .wt.toml. The approvals live
// in approvals.toml, next to the user's configuration file, which only
// "wt hook approve" writes.
//
// Decision/effect boundary: Hash, Set, Parse and Marshal are pure. Load and
// Save are the only functions that touch the filesystem.
package approval
