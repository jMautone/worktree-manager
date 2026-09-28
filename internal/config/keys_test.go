package config

import (
	"strings"
	"testing"
)

func TestRegistryDeclaresTheKeysOfThisChange(t *testing.T) {
	reg := Keys()
	for _, tc := range []struct {
		name   string
		def    any
		inRepo bool
	}{
		{"default_base", "", true},
		{"worktree_path", "{repo_parent}/{repo}.worktrees/{branch|sanitize}", true},
	} {
		k, ok := reg.Lookup(tc.name)
		if !ok {
			t.Errorf("key %q is not registered", tc.name)
			continue
		}
		if k.Default != tc.def || k.InRepo != tc.inRepo {
			t.Errorf("%s: default %q, InRepo %v; want %q, %v", tc.name, k.Default, k.InRepo, tc.def, tc.inRepo)
		}
		if _, err := k.Validate(k.Default); err != nil {
			t.Errorf("%s: its own default does not validate: %v", tc.name, err)
		}
	}
	if len(reg) != 2 {
		t.Errorf("registry has %d keys, want exactly 2", len(reg))
	}
	if _, ok := reg.Lookup("nope"); ok {
		t.Error("Lookup found an unknown key")
	}
}

func TestKeyValidation(t *testing.T) {
	reg := Keys()
	for _, tc := range []struct {
		key     string
		value   any
		wantErr string
	}{
		{"default_base", "origin/develop", ""},
		{"default_base", "", ""},
		{"default_base", int64(42), "expected a string"},
		{"default_base", true, "expected a string"},
		{"default_base", map[string]any{"a": "b"}, "expected a string"},
		{"worktree_path", "../{repo}-{branch|sanitize}", ""},
		{"worktree_path", "", "must not be empty"},
		{"worktree_path", []any{"a"}, "expected a string"},
	} {
		k, _ := reg.Lookup(tc.key)
		got, err := k.Validate(tc.value)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s = %#v: unexpected error %v", tc.key, tc.value, err)
		case tc.wantErr == "" && got != tc.value:
			t.Errorf("%s = %#v: validated to %#v", tc.key, tc.value, got)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s = %#v: error %v, want one containing %q", tc.key, tc.value, err, tc.wantErr)
		}
	}
}
