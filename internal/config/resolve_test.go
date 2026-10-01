package config

import (
	"errors"
	"strings"
	"testing"
)

// testRegistry adds a key that repository files may not set, which the real
// registry of this change does not have.
func testRegistry() Registry {
	return append(Keys(), Key{
		Name:     "secret_cmd",
		Type:     "string",
		Default:  "none",
		InRepo:   false,
		Validate: validateString,
	})
}

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func mustResolve(t *testing.T, reg Registry, layers ...Layer) *Config {
	t.Helper()
	cfg, err := Resolve(reg, layers...)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return cfg
}

func assertValue(t *testing.T, cfg *Config, key string, value any, source Source) {
	t.Helper()
	v, ok := cfg.Get(key)
	if !ok {
		t.Fatalf("%s: missing from resolved config", key)
	}
	if v.Value != value || v.Source != source {
		t.Errorf("%s = %#v from %s, want %#v from %s", key, v.Value, v.Source, value, source)
	}
}

func TestResolveDefaults(t *testing.T) {
	cfg := mustResolve(t, testRegistry())
	assertValue(t, cfg, "default_base", "", SourceDefault)
	assertValue(t, cfg, "worktree_path", "{repo_parent}/{repo}.worktrees/{branch|sanitize}", SourceDefault)
	if len(cfg.Values) != 3 || cfg.Values[0].Key != "default_base" || cfg.Values[2].Key != "secret_cmd" {
		t.Errorf("values not in registry order: %+v", cfg.Values)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", cfg.Warnings)
	}
}

func TestResolvePrecedence(t *testing.T) {
	reg := testRegistry()
	user := Layer{Source: SourceUser, Origin: "/u/config.toml", Values: map[string]any{"default_base": "origin/main"}}
	repo := Layer{Source: SourceRepo, Origin: "/r/.wt.toml", Values: map[string]any{"default_base": "origin/develop"}}

	t.Run("user beats default", func(t *testing.T) {
		assertValue(t, mustResolve(t, reg, user), "default_base", "origin/main", SourceUser)
	})
	t.Run("repo beats user", func(t *testing.T) {
		assertValue(t, mustResolve(t, reg, user, repo), "default_base", "origin/develop", SourceRepo)
	})
	t.Run("env beats files", func(t *testing.T) {
		e := EnvLayer(reg, env(map[string]string{"WT_DEFAULT_BASE": "origin/release"}))
		cfg := mustResolve(t, reg, user, repo, e)
		assertValue(t, cfg, "default_base", "origin/release", SourceEnv)
		if v, _ := cfg.Get("default_base"); v.Origin != "WT_DEFAULT_BASE" {
			t.Errorf("env origin = %q, want WT_DEFAULT_BASE", v.Origin)
		}
	})
	t.Run("precedence does not depend on argument order", func(t *testing.T) {
		e := EnvLayer(reg, env(map[string]string{"WT_DEFAULT_BASE": "origin/release"}))
		assertValue(t, mustResolve(t, reg, e, repo, user), "default_base", "origin/release", SourceEnv)
		assertValue(t, mustResolve(t, reg, repo, user), "default_base", "origin/develop", SourceRepo)
	})
	t.Run("empty env var is unset", func(t *testing.T) {
		e := EnvLayer(reg, env(map[string]string{"WT_DEFAULT_BASE": ""}))
		assertValue(t, mustResolve(t, reg, user, e), "default_base", "origin/main", SourceUser)
	})
	t.Run("unrelated WT_ vars are ignored", func(t *testing.T) {
		e := EnvLayer(reg, env(map[string]string{"WT_NOPE": "x", "WT_CONFIG": "/x"}))
		cfg := mustResolve(t, reg, e)
		if len(cfg.Warnings) != 0 || len(cfg.Values) != len(reg) {
			t.Errorf("unrelated WT_ vars had an effect: %+v", cfg)
		}
	})
}

func TestResolveUnknownKeyWarns(t *testing.T) {
	user := Layer{Source: SourceUser, Origin: "/u/config.toml", Values: map[string]any{"colour": "blue", "default_base": "x"}}
	cfg := mustResolve(t, testRegistry(), user)
	assertValue(t, cfg, "default_base", "x", SourceUser)
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], `"colour"`) || !strings.Contains(cfg.Warnings[0], "/u/config.toml") {
		t.Errorf("warnings = %q, want one naming colour and the file", cfg.Warnings)
	}
}

func TestResolveKeyNotAllowedInRepoIsIgnoredWithWarning(t *testing.T) {
	repo := Layer{Source: SourceRepo, Origin: "/r/.wt.toml", Values: map[string]any{"secret_cmd": "rm -rf ~", "default_base": "origin/develop"}}
	cfg := mustResolve(t, testRegistry(), repo)
	assertValue(t, cfg, "secret_cmd", "none", SourceDefault)
	assertValue(t, cfg, "default_base", "origin/develop", SourceRepo)
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], `"secret_cmd"`) || !strings.Contains(cfg.Warnings[0], ".wt.toml") {
		t.Errorf("warnings = %q, want one naming secret_cmd and .wt.toml", cfg.Warnings)
	}

	// The same key is fine in the user file.
	user := Layer{Source: SourceUser, Origin: "/u/config.toml", Values: map[string]any{"secret_cmd": "echo"}}
	assertValue(t, mustResolve(t, testRegistry(), user), "secret_cmd", "echo", SourceUser)
}

func TestResolveInvalidValueFails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		layer Layer
		want  []string
	}{
		{"wrong type in user file",
			Layer{Source: SourceUser, Origin: "/u/config.toml", Values: map[string]any{"default_base": int64(42)}},
			[]string{`"default_base"`, "/u/config.toml"}},
		{"wrong type in repo file",
			Layer{Source: SourceRepo, Origin: "/r/.wt.toml", Values: map[string]any{"worktree_path": true}},
			[]string{`"worktree_path"`, "/r/.wt.toml"}},
		{"constraint violated",
			Layer{Source: SourceUser, Origin: "/u/config.toml", Values: map[string]any{"worktree_path": ""}},
			[]string{`"worktree_path"`, "/u/config.toml", "must not be empty"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(testRegistry(), tc.layer)
			if err == nil {
				t.Fatal("no error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not mention %q", err, w)
				}
			}
		})
	}
}

var errBadNumber = errors.New("not a number")

func TestResolveInvalidValueInEnvNamesTheVariable(t *testing.T) {
	reg := Registry{{Name: "n", Type: "number", Default: "1", Validate: func(v any) (any, error) {
		if v != "1" && v != "2" {
			return nil, errBadNumber
		}
		return v, nil
	}}}
	_, err := Resolve(reg, EnvLayer(reg, env(map[string]string{"WT_N": "x"})))
	if err == nil || !strings.Contains(err.Error(), "WT_N") || !strings.Contains(err.Error(), `"n"`) {
		t.Errorf("error = %v, want one naming WT_N and the key", err)
	}
}
