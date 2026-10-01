package config

import (
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/shell"
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

// protocolCollisions returns the protocol variables of the shell integration
// that a key in reg would also read as WT_<KEY>.
func protocolCollisions(reg Registry) []string {
	var hits []string
	for _, k := range reg {
		for _, v := range []string{shell.DirectiveVar, shell.PreviousVar} {
			if EnvVar(k.Name) == v {
				hits = append(hits, k.Name+" -> "+v)
			}
		}
	}
	return hits
}

// The function passes WT_DIRECTIVE_CD_FILE and WT_PREVIOUS_DIR to the binary.
// They share the WT_ prefix with the environment layer, so a key named
// previous_dir would read the shell's previous directory as its value.
func TestNoKeyCollidesWithTheShellProtocol(t *testing.T) {
	if hits := protocolCollisions(Keys()); len(hits) > 0 {
		t.Errorf("keys collide with shell protocol variables: %q", hits)
	}
}

func TestProtocolCollisionsDetectsACollidingKey(t *testing.T) {
	reg := append(Keys(), Key{Name: "previous_dir", Type: "string", Default: "", Validate: validateString})
	if hits := protocolCollisions(reg); len(hits) != 1 || !strings.Contains(hits[0], shell.PreviousVar) {
		t.Errorf("collisions = %q, want previous_dir -> %s", hits, shell.PreviousVar)
	}
}
