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
	"runtime/debug"

	"github.com/jMautone/worktree-manager/internal/cli"
	"github.com/jMautone/worktree-manager/internal/term"
)

// version is set by release builds via -ldflags "-X main.version=...". When it
// is empty, cli.ResolveVersion falls back to the module version in the binary.
var version string

func main() {
	os.Exit(cli.Run(context.Background(), cli.Env{
		Args:    os.Args[1:],
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		Environ: os.Environ(),
		Getwd:   os.Getwd,
		Chdir:   os.Chdir,
		GOOS:    runtime.GOOS,
		IsTTY:   term.IsTerminal(os.Stdout) && term.EnableANSI(os.Stdout),
		Version: cli.ResolveVersion(version, moduleVersion()),
	}))
}

// moduleVersion is the main module version Go recorded in the binary, or "".
func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return ""
}
