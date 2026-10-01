// Command git-wt is the wt binary: a git worktree manager for multi-repo workspaces.
//
// The binary is named git-wt so that git exposes it as `git wt <subcommand>`.
// Users normally invoke it through the `wt` shell function installed by
// `wt shell init`, because a binary cannot change its parent shell's working
// directory. See docs/design/product.md.
//
// main only builds the real environment; everything else is internal/cli.
package main

import (
	"context"
	"os"
	"runtime"

	"github.com/jMautone/worktree-manager/internal/cli"
	"github.com/jMautone/worktree-manager/internal/term"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	os.Exit(cli.Run(context.Background(), cli.Env{
		Args:    os.Args[1:],
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		Environ: os.Environ(),
		Getwd:   os.Getwd,
		GOOS:    runtime.GOOS,
		IsTTY:   term.IsTerminal(os.Stdout) && term.EnableANSI(os.Stdout),
		Version: version,
	}))
}
