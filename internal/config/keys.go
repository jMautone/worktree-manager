package config

import (
	"errors"
	"fmt"
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

// Keys returns the registry of every key wt knows.
func Keys() Registry {
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
