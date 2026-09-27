// Package workspace discovers repositories under the configured roots and
// resolves a name to a repository, a worktree, or an ambiguity.
//
// This is the product's differentiator: unlike comparable tools, wt does not
// assume the user is already inside a repository. Ambiguity is a first-class
// outcome, not an error to paper over -- it has its own exit code.
package workspace
