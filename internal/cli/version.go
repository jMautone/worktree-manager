package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// DevVersion is the version reported when the build recorded none.
const DevVersion = "0.0.0-dev"

// ResolveVersion picks the version wt reports. injected is what release builds
// set with -ldflags "-X main.version=...". module is the main module version Go
// records in the binary: go install …@vX.Y.Z records the tag, a go build in a
// checkout records a tag or a pseudo-version, and "(devel)" means none. The
// leading "v" is dropped, so every source reads like a release build.
func ResolveVersion(injected, module string) string {
	switch {
	case injected != "":
		return injected
	case module != "" && module != "(devel)":
		return strings.TrimPrefix(module, "v")
	default:
		return DevVersion
	}
}

func (a *app) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  noArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			return a.printVersion()
		}),
	}
}

// printVersion prints the version. It reads no configuration, so it works
// even when the configuration is broken.
func (a *app) printVersion() error {
	commit := buildCommit()
	if a.json {
		return a.writeJSON(struct {
			Schema  string `json:"schema"`
			Version string `json:"version"`
			Commit  string `json:"commit"`
			OS      string `json:"os"`
			Arch    string `json:"arch"`
		}{"wt.version.v1", a.env.Version, commit, runtime.GOOS, runtime.GOARCH})
	}
	line := "wt " + a.env.Version
	if commit != "" {
		line += " (" + commit[:min(12, len(commit))] + ")"
	}
	_, err := fmt.Fprintln(a.env.Stdout, line)
	return err
}

// buildCommit is the source revision the binary was built from, or "" when
// the build did not record it.
func buildCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
