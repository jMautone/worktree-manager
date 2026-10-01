module github.com/jMautone/worktree-manager

go 1.27

// v0.9.0 is the PowerShell line, renamed to the tag powershell-v0.9.0. It has
// no Go code, but the module proxy cached it, so @latest resolves to it until
// a higher release exists. The retraction takes effect from v1.0.0 on.
// See docs/decisions/0002-versionado-y-releases.md.
retract v0.9.0
