// Command git-wt is the wt binary: a git worktree manager for multi-repo workspaces.
//
// The binary is named git-wt so that git exposes it as `git wt <subcommand>`.
// Users normally invoke it through the `wt` shell function installed by
// `wt shell init`, because a binary cannot change its parent shell's working
// directory. See docs/design/product.md.
//
// This is the bootstrap scaffold: the command surface arrives with the
// walking-skeleton change (M1). See openspec/changes/.
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wt:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println(version)
		return nil
	}
	return fmt.Errorf("not implemented yet: v1 is being built, see docs/design/product.md (stable line: git checkout v0.9.x)")
}
