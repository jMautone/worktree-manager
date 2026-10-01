package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage:
  relcheck pr --branch <head-ref> --title <title> [--base <ref>] [--root <dir>]
      Check a pull request. Prints the version its merge publishes.
  relcheck merge [--rev <rev>] [--root <dir>]
      Read the version a squash commit on main publishes and re-check it.
      Prints the version, or nothing when the commit publishes nothing.
  relcheck notes --version <vX.Y.0> [--root <dir>]
      Print the CHANGELOG.md section of a final release.
`

// run executes a subcommand and returns the process exit code: 0 ok,
// 1 a convention is violated or git failed, 2 usage error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	commands := map[string]func([]string, io.Writer) error{
		"pr":    runPR,
		"merge": runMerge,
		"notes": runNotes,
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if err := cmd(args[1:], stdout); err != nil {
		var usageErr usageError
		if errors.As(err, &usageErr) {
			fmt.Fprintln(stderr, "relcheck:", err)
			fmt.Fprint(stderr, usage)
			return 2
		}
		for _, line := range strings.Split(err.Error(), "\n") {
			fmt.Fprintln(stderr, "relcheck:", line)
		}
		return 1
	}
	return 0
}

func newFlags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

// usageError is a malformed command line: run prints the usage and exits 2.
type usageError struct{ error }

// parseFlags parses args. Unknown flags, positional arguments and missing
// required flags are usage errors.
func parseFlags(flags *flag.FlagSet, args []string, required ...string) error {
	if err := flags.Parse(args); err != nil {
		return usageError{err}
	}
	if flags.NArg() > 0 {
		return usageError{fmt.Errorf("unexpected argument %q", flags.Arg(0))}
	}
	for _, name := range required {
		if flags.Lookup(name).Value.String() == "" {
			return usageError{fmt.Errorf("--%s is required", name)}
		}
	}
	return nil
}

func runPR(args []string, stdout io.Writer) error {
	flags := newFlags("pr")
	branch := flags.String("branch", "", "head branch of the pull request")
	title := flags.String("title", "", "title of the pull request")
	base := flags.String("base", "origin/main", "ref whose history holds the published versions")
	root := flags.String("root", ".", "repository root")
	if err := parseFlags(flags, args, "branch", "title"); err != nil {
		return err
	}

	b, berr := ParseBranch(*branch)
	t, terr := ParseTitle(*title)
	if err := errors.Join(berr, terr); err != nil {
		return err
	}
	subjects, err := gitSubjects(*root, *base)
	if err != nil {
		return err
	}
	f := Facts{}
	if f.Published, err = PublishedVersions(subjects); err != nil {
		return err
	}
	switch b.Kind {
	case ChangeBranch:
		if f.ChangeExists, err = changeExists(*root, b.Change); err != nil {
			return err
		}
	case ReleaseBranch:
		content, err := readChangelog(*root)
		if err != nil {
			return err
		}
		_, f.ChangelogSection = ChangelogSection(content, b.Release)
	}

	v, errs := CheckPR(b, t, f)
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if v == nil {
		fmt.Fprintln(stdout, "ok: publishes nothing")
	} else {
		fmt.Fprintln(stdout, "ok: publishes", v)
	}
	return nil
}

func runMerge(args []string, stdout io.Writer) error {
	flags := newFlags("merge")
	rev := flags.String("rev", "HEAD", "the squash commit pushed to main")
	root := flags.String("root", ".", "repository root")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	subject, err := git(*root, "log", "-1", "--format=%s", *rev)
	if err != nil {
		return err
	}
	v, ok, err := SubjectVersion(strings.TrimSpace(subject))
	if err != nil || !ok {
		return err // no suffix: the commit publishes nothing
	}
	// Everything main published before this commit. On a re-run after a
	// partial failure the commit itself is excluded, so the check still passes.
	subjects, err := gitSubjects(*root, *rev+"^")
	if err != nil {
		return err
	}
	published, err := PublishedVersions(subjects)
	if err != nil {
		return err
	}
	if err := NextCheck(v, published); err != nil {
		return err
	}
	if !v.IsAlpha() {
		content, err := readChangelog(*root)
		if err != nil {
			return err
		}
		if _, ok := ChangelogSection(content, v); !ok {
			return fmt.Errorf("CHANGELOG.md has no \"## [%s]\" section for the release notes", strings.TrimPrefix(v.String(), "v"))
		}
	}
	// A tag already on this commit means an earlier run got this far.
	if tagged, err := git(*root, "rev-parse", "-q", "--verify", "refs/tags/"+v.String()+"^{commit}"); err == nil {
		commit, err := git(*root, "rev-parse", *rev+"^{commit}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(tagged) != strings.TrimSpace(commit) {
			return fmt.Errorf("tag %s already exists on another commit", v)
		}
	}
	fmt.Fprintln(stdout, v)
	return nil
}

func runNotes(args []string, stdout io.Writer) error {
	flags := newFlags("notes")
	version := flags.String("version", "", "final version, vX.Y.0")
	root := flags.String("root", ".", "repository root")
	if err := parseFlags(flags, args, "version"); err != nil {
		return err
	}
	v, err := ParseVersion(*version)
	if err != nil {
		return err
	}
	if v.IsAlpha() {
		return fmt.Errorf("%s is an alpha: its release notes are generated from the merged titles; notes only reads the CHANGELOG section of a final, vX.Y.0", v)
	}
	content, err := readChangelog(*root)
	if err != nil {
		return err
	}
	body, ok := ChangelogSection(content, v)
	if !ok {
		return fmt.Errorf("CHANGELOG.md has no \"## [%s]\" section with release notes", strings.TrimPrefix(v.String(), "v"))
	}
	fmt.Fprintln(stdout, body)
	return nil
}

// readChangelog returns CHANGELOG.md, or "" when the file does not exist.
func readChangelog(root string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(content), err
}

// changeExists reports whether openspec/changes/<change>/ or its archive exists.
func changeExists(root, change string) (bool, error) {
	changes := filepath.Join(root, "openspec", "changes")
	if change != "archive" {
		if info, err := os.Stat(filepath.Join(changes, change)); err == nil && info.IsDir() {
			return true, nil
		}
	}
	entries, err := os.ReadDir(filepath.Join(changes, "archive"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() && ArchivedAs(e.Name(), change) {
			return true, nil
		}
	}
	return false, nil
}

func gitSubjects(root, rev string) ([]string, error) {
	out, err := git(root, "log", "--format=%s", rev)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n"), nil
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}
