module github.com/jMautone/worktree-manager

go 1.27

require (
	github.com/pelletier/go-toml/v2 v2.4.3
	github.com/spf13/cobra v1.10.2
	golang.org/x/sys v0.48.0
	golang.org/x/term v0.46.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

// v0.9.0 is the PowerShell line, renamed to the tag powershell-v0.9.0. It has
// no Go code, but the module proxy cached it, so @latest resolves to it until
// a higher release exists. The retraction takes effect from v1.0.0 on.
// See docs/decisions/0002-versionado-y-releases.md.
retract v0.9.0
