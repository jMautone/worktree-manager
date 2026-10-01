package config

import (
	"errors"
	"fmt"
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
	// Validate checks a raw value (as decoded from TOML, or the string of an
	// environment variable) and returns the value to use.
	Validate func(any) (any, error)
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
			Name:     "worktree_path",
			Type:     "string",
			Default:  "{repo_parent}/{repo}.worktrees/{branch|sanitize}",
			InRepo:   true,
			Validate: validateNonEmptyString,
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
