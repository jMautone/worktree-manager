package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jMautone/worktree-manager/internal/config"
)

func (a *app) configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the configuration",
		Long: `Inspect the configuration.

Values come from, lowest to highest precedence: defaults, the user file,
the repository's .wt.toml (allowlisted keys only) and WT_<KEY> environment
variables. Edit the files by hand; "wt config path" shows where they are.`,
		Args: unknownSubcommand,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		}),
	}
	cmd.SuggestionsMinimumDistance = 2
	cmd.AddCommand(a.configPathCommand(), a.configListCommand(), a.configGetCommand())
	return cmd
}

// repoRootForConfig is the root whose .wt.toml applies, or "" outside any
// worktree.
func (a *app) repoRootForConfig(ctx context.Context) (string, error) {
	dir, err := a.workdir()
	if err != nil {
		return "", err
	}
	repo, err := a.optionalRepository(ctx, dir)
	if err != nil {
		return "", err
	}
	return repo.root(), nil
}

type configFileJSON struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

func (a *app) configPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show where the configuration files are",
		Args:  noArgs,
		// Does not parse the files: it must work when they are broken.
		RunE: action(func(cmd *cobra.Command, args []string) error {
			root, err := a.repoRootForConfig(cmd.Context())
			if err != nil {
				return err
			}
			userPath, err := a.userFile()
			if err != nil {
				return err
			}
			user := configFileJSON{Path: userPath, Exists: fileExists(userPath)}
			var repo *configFileJSON
			if root != "" {
				p := filepath.Join(root, config.RepoFileName)
				repo = &configFileJSON{Path: p, Exists: fileExists(p)}
			}
			if a.json {
				return a.writeJSON(struct {
					Schema string          `json:"schema"`
					User   configFileJSON  `json:"user"`
					Repo   *configFileJSON `json:"repo"`
				}{"wt.config.path.v1", user, repo})
			}
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "user: %s (%s)\n", user.Path, existence(user.Exists))
			if repo != nil {
				fmt.Fprintf(&buf, "repo: %s (%s)\n", repo.Path, existence(repo.Exists))
			} else {
				fmt.Fprintln(&buf, "repo: none (not inside a worktree)")
			}
			_, err = a.env.Stdout.Write(buf.Bytes())
			return err
		}),
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func existence(exists bool) string {
	if exists {
		return "exists"
	}
	return "not found"
}

// configValueJSON is one key in wt.config.list.v1 and wt.config.get.v1.
type configValueJSON struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

func (a *app) configListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every key with its effective value and where it comes from",
		Args:  noArgs,
		RunE: action(func(cmd *cobra.Command, args []string) error {
			root, err := a.repoRootForConfig(cmd.Context())
			if err != nil {
				return err
			}
			cfg, err := a.loadConfig(root)
			if err != nil {
				return err
			}
			if a.json {
				keys := make([]configValueJSON, 0, len(cfg.Values))
				for _, v := range cfg.Values {
					keys = append(keys, configValueJSON{v.Key, v.Value, string(v.Source)})
				}
				return a.writeJSON(struct {
					Schema string            `json:"schema"`
					Keys   []configValueJSON `json:"keys"`
				}{"wt.config.list.v1", keys})
			}
			rows := [][]cell{{
				{text: "KEY", style: styleBold},
				{text: "VALUE", style: styleBold},
				{text: "SOURCE", style: styleBold},
			}}
			for _, v := range cfg.Values {
				rows = append(rows, []cell{{text: v.Key}, {text: displayValue(v.Value)}, {text: string(v.Source), style: styleDim}})
			}
			var buf bytes.Buffer
			if err := renderTable(&buf, rows, a.color()); err != nil {
				return err
			}
			_, err = a.env.Stdout.Write(buf.Bytes())
			return err
		}),
	}
}

// displayValue shows a value in the `config list` table, where an empty
// string would otherwise be an invisible blank. A list is shown as a TOML
// array, as it is written in the file.
func displayValue(v any) string {
	if list, ok := v.([]string); ok {
		quoted := make([]string, len(list))
		for i, e := range list {
			quoted[i] = strconv.Quote(e)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	}
	s := fmt.Sprint(v)
	if s == "" {
		return `""`
	}
	return s
}

func (a *app) configGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print the effective value of a key",
		Args:  exactArgs("key"),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			reg := config.Keys(a.env.GOOS)
			if _, ok := reg.Lookup(args[0]); !ok {
				return &Error{
					Code:  ExitUsage,
					Msg:   fmt.Sprintf("unknown configuration key %q", args[0]),
					Hints: []string{"known keys: " + strings.Join(reg.Names(), ", ")},
				}
			}
			root, err := a.repoRootForConfig(cmd.Context())
			if err != nil {
				return err
			}
			cfg, err := a.loadConfig(root)
			if err != nil {
				return err
			}
			v, _ := cfg.Get(args[0])
			if a.json {
				return a.writeJSON(struct {
					Schema string `json:"schema"`
					configValueJSON
				}{"wt.config.get.v1", configValueJSON{v.Key, v.Value, string(v.Source)}})
			}
			// A list prints one element per line, and nothing when empty,
			// so that a script can read it line by line.
			if list, ok := v.Value.([]string); ok {
				var buf bytes.Buffer
				for _, e := range list {
					fmt.Fprintln(&buf, e)
				}
				_, err = a.env.Stdout.Write(buf.Bytes())
				return err
			}
			_, err = fmt.Fprintln(a.env.Stdout, v.Value)
			return err
		}),
	}
}
