package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jMautone/worktree-manager/internal/template"
)

// Key declares one configuration key. Every key states its type, its default
// and whether a repository's .wt.toml may set it.
type Key struct {
	Name    string
	Type    string // for messages and docs, e.g. "string"
	Default any
	// InRepo allows the key in .wt.toml. The repository file comes from a
	// possibly untrusted clone, so this is an allowlist.
	InRepo bool
	// Validate checks a raw value (as decoded from TOML, or what FromEnv
	// returned) and returns the value to use.
	Validate func(any) (any, error)
	// FromEnv converts the text of WT_<KEY> before Validate; nil passes the
	// text as is. Files are not converted: in TOML a value has the key's type.
	FromEnv func(string) (any, error)
}

// Registry is the ordered set of known keys. It is passed to Resolve rather
// than read from a global, so tests can declare keys the product does not
// have yet.
type Registry []Key

// Lookup returns the key with the given name.
func (r Registry) Lookup(name string) (Key, bool) {
	for _, k := range r {
		if k.Name == name {
			return k, true
		}
	}
	return Key{}, false
}

// Names returns the key names in registry order.
func (r Registry) Names() []string {
	names := make([]string, len(r))
	for i, k := range r {
		names[i] = k.Name
	}
	return names
}

// Keys returns the registry of every key wt knows. goos decides what an
// absolute path is and how a list is split in the environment, so the rules
// of every OS are testable from any OS.
func Keys(goos string) Registry {
	return Registry{
		{
			Name: "default_base",
			Type: "string",
			// Empty means the repository's default branch, resolved by the
			// command that consumes the key.
			Default:  "",
			InRepo:   true,
			Validate: validateString,
		},
		{
			Name: "worktree_path",
			Type: "template",
			// {name}, not {branch}: wt cd <name> finds what wt create made,
			// whatever -b or branch_prefix chose for the branch.
			Default:  "{repo_parent}/{repo}.worktrees/{name|sanitize}",
			InRepo:   true,
			Validate: validateTemplate,
		},
		{
			Name:     "branch_prefix",
			Type:     "string",
			Default:  "",
			InRepo:   true,
			Validate: validateString,
		},
		{
			Name:     "fetch_before_create",
			Type:     "boolean",
			Default:  true,
			InRepo:   true,
			Validate: validateBool,
			FromEnv:  parseBoolEnv,
		},
		{
			Name: "create_cd",
			Type: "boolean",
			// Whether wt create moves the shell is a preference of whoever
			// uses the terminal, not of the repository.
			Default:  true,
			InRepo:   false,
			Validate: validateBool,
			FromEnv:  parseBoolEnv,
		},
		{
			Name: "repos_root",
			Type: "path list",
			// No roots, no workspace: wt behaves as it did before it had one.
			Default: []string{},
			// Where wt searches is the user's machine, not the repository's.
			InRepo:   false,
			Validate: validatePathList(goos),
			FromEnv:  splitPathList(goos),
		},
		{
			Name:     "repos_depth",
			Type:     "integer",
			Default:  1,
			InRepo:   false,
			Validate: validateIntRange(1, 3),
			FromEnv:  parseIntEnv,
		},
	}
}

func validateString(v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("expected a string, got %s", describe(v))
	}
	return s, nil
}

func validateNonEmptyString(v any) (any, error) {
	s, err := validateString(v)
	if err != nil {
		return nil, err
	}
	if s == "" {
		return nil, errors.New("must not be empty")
	}
	return s, nil
}

// validateTemplate accepts a worktree_path template. It returns the text,
// not the parsed template: wt config get prints the template as written, and
// the command that renders it parses it again.
func validateTemplate(v any) (any, error) {
	s, err := validateNonEmptyString(v)
	if err != nil {
		return nil, err
	}
	if _, err := template.Parse(s.(string), template.WorktreePathVars); err != nil {
		return nil, err
	}
	return s, nil
}

func validateBool(v any) (any, error) {
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("expected a boolean, got %s", describe(v))
	}
	return b, nil
}

// parseBoolEnv reads a boolean from the environment: true and false,
// ignoring case, or 1 and 0.
func parseBoolEnv(s string) (any, error) {
	switch {
	case s == "1" || strings.EqualFold(s, "true"):
		return true, nil
	case s == "0" || strings.EqualFold(s, "false"):
		return false, nil
	}
	return nil, fmt.Errorf("expected true, false, 1 or 0, got %q", s)
}

// validatePathList accepts an array of paths, each absolute for goos or
// starting with ~. Elements are kept as written: the command that uses the
// key expands ~. The value is a []string, never nil, so an empty list is []
// in JSON.
func validatePathList(goos string) func(any) (any, error) {
	return func(v any) (any, error) {
		var elems []any
		switch v := v.(type) {
		case []any:
			elems = v
		case []string:
			// What Validate returned, as in a key's default.
			for _, s := range v {
				elems = append(elems, s)
			}
		case string:
			return nil, fmt.Errorf("expected an array of strings, got a string; write it as [%q]", v)
		default:
			return nil, fmt.Errorf("expected an array of strings, got %s", describe(v))
		}
		paths := make([]string, 0, len(elems))
		for i, e := range elems {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("expected an array of strings, element %d is %s", i+1, describe(e))
			}
			if !isAbs(goos, s) && !hasTilde(goos, s) {
				return nil, fmt.Errorf("%q is not an absolute path and does not start with %s", s, tildePrefixes(goos))
			}
			paths = append(paths, s)
		}
		return paths, nil
	}
}

// hasTilde reports whether s is ~ or starts with ~/ (or ~\ on windows).
func hasTilde(goos, s string) bool {
	return s == "~" || strings.HasPrefix(s, "~/") || goos == "windows" && strings.HasPrefix(s, `~\`)
}

// tildePrefixes names, for messages, the forms hasTilde accepts.
func tildePrefixes(goos string) string {
	if goos == "windows" {
		return `~/ or ~\`
	}
	return "~/"
}

// splitPathList reads a list of paths from the environment, split where
// PATH is: at : on macOS and Linux, and at ; on windows, where C:\x has a
// colon. Empty elements are dropped. The result goes through Validate like
// an array from a file.
func splitPathList(goos string) func(string) (any, error) {
	sep := ":"
	if goos == "windows" {
		sep = ";"
	}
	return func(s string) (any, error) {
		elems := []any{}
		for _, e := range strings.Split(s, sep) {
			if e != "" {
				elems = append(elems, e)
			}
		}
		return elems, nil
	}
}

// validateIntRange accepts an integer from lo to hi. TOML decodes integers
// as int64; the value kept is an int.
func validateIntRange(lo, hi int) func(any) (any, error) {
	return func(v any) (any, error) {
		var n int64
		switch v := v.(type) {
		case int64:
			n = v
		case int:
			// What Validate returned, as in a key's default.
			n = int64(v)
		default:
			return nil, fmt.Errorf("expected an integer, got %s", describe(v))
		}
		if n < int64(lo) || n > int64(hi) {
			return nil, fmt.Errorf("must be from %d to %d, got %d", lo, hi, n)
		}
		return int(n), nil
	}
}

// parseIntEnv reads an integer written in decimal digits from the
// environment.
func parseIntEnv(s string) (any, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("expected an integer, got %q", s)
	}
	return n, nil
}

// describe names the TOML type of a decoded value.
func describe(v any) string {
	switch v.(type) {
	case string:
		return "a string"
	case int64:
		return "an integer"
	case float64:
		return "a float"
	case bool:
		return "a boolean"
	case []any:
		return "an array"
	case map[string]any:
		return "a table"
	default:
		return fmt.Sprintf("%T", v)
	}
}
