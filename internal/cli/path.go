package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const pathLong = `Print the path that "wt cd <target>" would move the shell to.

<target> is resolved as in "wt cd", with the same errors and exit codes,
except that - is a name like any other. wt path changes nothing and does
not need the wt shell function, so it works in scripts and as an
argument, as in code "$(wt path api:feat)".`

func (a *app) pathCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "path <target>",
		Short:             "Print the path of a worktree or a repository",
		Long:              pathLong,
		Args:              exactArgs("target"),
		ValidArgsFunction: a.completeTargets(modeNavigate, canEnter),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			dir, err := a.workdir()
			if err != nil {
				return err
			}
			res, _, err := a.resolve(cmd.Context(), dir, args[0], modeNavigate)
			if err != nil {
				return err
			}
			if err := mustExist(res.Path); err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(struct {
					Schema string `json:"schema"`
					Path   string `json:"path"`
				}{"wt.path.v1", res.Path})
			}
			_, err = fmt.Fprintln(a.env.Stdout, res.Path)
			return err
		}),
	}
}
