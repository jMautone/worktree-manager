// Package template is the one template engine used by worktree paths, hooks,
// launchers, aliases and list columns.
//
// Variables: {repo} {repo_parent} {repo_path} {branch} {name} {path} {base}
// {default_branch} {upstream} {owner} {args} {vars.X}
// Filters: sanitize, hash_port, codename(n), lower
//
// The engine exists from the first milestone on purpose: without sanitize a
// branch such as feature/abc1 has no valid path on any filesystem.
package template
