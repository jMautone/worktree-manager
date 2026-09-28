package config

import (
	"errors"
	"strings"
)

// RepoFileName is the repository configuration file, read from the root of
// the current worktree.
const RepoFileName = ".wt.toml"

// UserFile returns the path of the user configuration file for goos. It does
// not touch the filesystem, so the paths of every OS are testable from any OS.
//
//   - WT_CONFIG, when non-empty, wins on every OS.
//   - macOS and Linux: $XDG_CONFIG_HOME/wt/config.toml when XDG_CONFIG_HOME is
//     an absolute path, else ~/.config/wt/config.toml. macOS deliberately
//     does not use ~/Library/Application Support: ~/.config is where terminal
//     developer tools (git, gh, starship) keep theirs.
//   - Windows: %APPDATA%\wt\config.toml.
func UserFile(goos string, getenv func(string) string, home string) (string, error) {
	if p := getenv("WT_CONFIG"); p != "" {
		return p, nil
	}
	if goos == "windows" {
		appdata := getenv("APPDATA")
		if appdata == "" {
			return "", errors.New("cannot locate the user configuration file: APPDATA is not set")
		}
		return join(goos, appdata, "wt", "config.toml"), nil
	}
	if xdg := getenv("XDG_CONFIG_HOME"); strings.HasPrefix(xdg, "/") {
		return join(goos, xdg, "wt", "config.toml"), nil
	}
	if home == "" {
		return "", errors.New("cannot locate the user configuration file: HOME is not set")
	}
	return join(goos, home, ".config", "wt", "config.toml"), nil
}

// join joins path elements with goos's separator. filepath.Join would use the
// separator of the OS running the code.
func join(goos, base string, elem ...string) string {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	base = strings.TrimRight(base, `/\`)
	return base + sep + strings.Join(elem, sep)
}
