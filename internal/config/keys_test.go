package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/shell"
)

func TestRegistryDeclaresTheKeysOfThisChange(t *testing.T) {
	reg := Keys("darwin")
	for _, tc := range []struct {
		name   string
		def    any
		inRepo bool
	}{
		{"default_base", "", true},
		{"worktree_path", "{repo_parent}/{repo}.worktrees/{name|sanitize}", true},
		{"branch_prefix", "", true},
		{"fetch_before_create", true, true},
		{"create_cd", true, false},
		{"repos_root", []string{}, false},
		{"repos_depth", 1, false},
	} {
		k, ok := reg.Lookup(tc.name)
		if !ok {
			t.Errorf("key %q is not registered", tc.name)
			continue
		}
		if !reflect.DeepEqual(k.Default, tc.def) || k.InRepo != tc.inRepo {
			t.Errorf("%s: default %#v, InRepo %v; want %#v, %v", tc.name, k.Default, k.InRepo, tc.def, tc.inRepo)
		}
		if v, err := k.Validate(k.Default); err != nil || !reflect.DeepEqual(v, k.Default) {
			t.Errorf("%s: its own default validates to %#v, %v", tc.name, v, err)
		}
	}
	if len(reg) != 7 {
		t.Errorf("registry has %d keys, want exactly 7", len(reg))
	}
	if _, ok := reg.Lookup("nope"); ok {
		t.Error("Lookup found an unknown key")
	}
}

func TestKeyValidation(t *testing.T) {
	reg := Keys("darwin")
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
		{"worktree_path", "{repo_parent}/{ name | sanitize | lower }", ""},
		{"worktree_path", "", "must not be empty"},
		{"worktree_path", []any{"a"}, "expected a string"},
		{"worktree_path", "{repo_parent}/{nope}", `unknown variable "nope"`},
		{"worktree_path", "{repo_parent}/{name|upper}", `unknown filter "upper"`},
		{"worktree_path", "{repo_parent/x", `"{repo_parent/x"`},
		{"branch_prefix", "jm/", ""},
		{"branch_prefix", false, "expected a string"},
		{"fetch_before_create", false, ""},
		{"fetch_before_create", "false", "expected a boolean, got a string"},
		{"create_cd", true, ""},
		{"create_cd", int64(0), "expected a boolean, got an integer"},
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

// Scenarios of "List values": what an element of repos_root may be on each
// OS.
func TestPathListValidation(t *testing.T) {
	const notAbs = "is not an absolute path"
	for _, tc := range []struct {
		goos    string
		value   any
		wantErr string // "" accepts the value as written
	}{
		{"darwin", []any{"/x"}, ""},
		{"linux", []any{"/x"}, ""},
		{"windows", []any{"/x"}, notAbs},
		{"darwin", []any{"~"}, ""},
		{"windows", []any{"~"}, ""},
		{"darwin", []any{"~/x"}, ""},
		{"windows", []any{"~/x"}, ""},
		{"darwin", []any{`~\x`}, notAbs},
		{"linux", []any{`~\x`}, notAbs},
		{"windows", []any{`~\x`}, ""},
		{"darwin", []any{"~x"}, notAbs},
		{"windows", []any{`C:\x`}, ""},
		{"windows", []any{"C:/x"}, ""},
		{"windows", []any{`c:\x`}, ""},
		{"darwin", []any{`C:\x`}, notAbs},
		{"windows", []any{`\\srv\share`}, ""},
		{"linux", []any{`\\srv\share`}, notAbs},
		{"windows", []any{`\x`}, notAbs},
		{"windows", []any{"C:x"}, notAbs},
		{"darwin", []any{"x"}, notAbs},
		{"windows", []any{"x"}, notAbs},
		{"darwin", []any{""}, `""`},
		{"windows", []any{""}, `""`},
		{"darwin", []any{"/a", "~/b", "/Volumes/x"}, ""},
		{"darwin", []any{"/a", "GIT"}, `"GIT"`},
		{"darwin", []any{}, ""},
		{"darwin", []any{"/a", int64(1)}, "element 2 is an integer"},
		{"darwin", "~/GIT", `got a string; write it as ["~/GIT"]`},
		{"darwin", int64(1), "expected an array of strings, got an integer"},
		{"darwin", map[string]any{}, "expected an array of strings, got a table"},
	} {
		k, _ := Keys(tc.goos).Lookup("repos_root")
		got, err := k.Validate(tc.value)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: %#v: error %v, want one containing %q", tc.goos, tc.value, err, tc.wantErr)
			}
			continue
		}
		want := []string{}
		for _, e := range tc.value.([]any) {
			want = append(want, e.(string))
		}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %#v validated to %#v, %v; want %#v", tc.goos, tc.value, got, err, want)
		}
	}
}

// WT_REPOS_ROOT is split where PATH is on each OS; an element that is not a
// path fails naming the variable and the element.
func TestPathListFromTheEnvironment(t *testing.T) {
	for _, tc := range []struct {
		goos, value string
		want        []string
	}{
		{"darwin", "~/GIT:/Volumes/x", []string{"~/GIT", "/Volumes/x"}},
		{"linux", "::/a::~/b:", []string{"/a", "~/b"}},
		{"linux", ":", []string{}},
		{"windows", `C:\Repos;D:\Work`, []string{`C:\Repos`, `D:\Work`}},
		{"windows", `;C:\Repos;;~\GIT;`, []string{`C:\Repos`, `~\GIT`}},
	} {
		reg := Keys(tc.goos)
		cfg := mustResolve(t, reg, EnvLayer(reg, env(map[string]string{"WT_REPOS_ROOT": tc.value})))
		v, _ := cfg.Get("repos_root")
		if !reflect.DeepEqual(v.Value, tc.want) || v.Source != SourceEnv {
			t.Errorf("%s: WT_REPOS_ROOT=%s is %#v from %s, want %#v from env", tc.goos, tc.value, v.Value, v.Source, tc.want)
		}
	}

	reg := Keys("darwin")
	_, err := Resolve(reg, EnvLayer(reg, env(map[string]string{"WT_REPOS_ROOT": `/a:GIT`})))
	if err == nil || !strings.Contains(err.Error(), "WT_REPOS_ROOT") || !strings.Contains(err.Error(), `"GIT"`) {
		t.Errorf("WT_REPOS_ROOT=/a:GIT: error %v, want one naming the variable and the element", err)
	}
}

// Scenarios of "Integer values".
func TestReposDepth(t *testing.T) {
	k, _ := Keys("darwin").Lookup("repos_depth")
	for _, tc := range []struct {
		value   any
		want    int
		wantErr string
	}{
		{int64(0), 0, "from 1 to 3, got 0"},
		{int64(1), 1, ""},
		{int64(3), 3, ""},
		{int64(4), 0, "from 1 to 3, got 4"},
		{int64(-1), 0, "got -1"},
		{2.0, 0, "expected an integer, got a float"},
		{"2", 0, "expected an integer, got a string"},
	} {
		got, err := k.Validate(tc.value)
		switch {
		case tc.wantErr == "" && (err != nil || got != tc.want):
			t.Errorf("repos_depth = %#v: %#v, %v; want %d", tc.value, got, err, tc.want)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("repos_depth = %#v: error %v, want one containing %q", tc.value, err, tc.wantErr)
		}
	}

	reg := Keys("darwin")
	cfg := mustResolve(t, reg, EnvLayer(reg, env(map[string]string{"WT_REPOS_DEPTH": "3"})))
	assertValue(t, cfg, "repos_depth", 3, SourceEnv)
	for _, value := range []string{"two", "2.0", " 2", "4"} {
		_, err := Resolve(reg, EnvLayer(reg, env(map[string]string{"WT_REPOS_DEPTH": value})))
		if err == nil || !strings.Contains(err.Error(), "WT_REPOS_DEPTH") || !strings.Contains(err.Error(), `"repos_depth"`) {
			t.Errorf("WT_REPOS_DEPTH=%s: error %v, want one naming the variable and the key", value, err)
		}
	}
}

func TestParseBoolEnv(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want any
	}{
		{"true", true}, {"TRUE", true}, {"True", true}, {"1", true},
		{"false", false}, {"FALSE", false}, {"fAlSe", false}, {"0", false},
	} {
		if got, err := parseBoolEnv(tc.in); err != nil || got != tc.want {
			t.Errorf("parseBoolEnv(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{"yes", "no", "on", "2", " 1", "t"} {
		if _, err := parseBoolEnv(in); err == nil {
			t.Errorf("parseBoolEnv(%q) accepted it", in)
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
	if hits := protocolCollisions(Keys("darwin")); len(hits) > 0 {
		t.Errorf("keys collide with shell protocol variables: %q", hits)
	}
}

func TestProtocolCollisionsDetectsACollidingKey(t *testing.T) {
	reg := append(Keys("darwin"), Key{Name: "previous_dir", Type: "string", Default: "", Validate: validateString})
	if hits := protocolCollisions(reg); len(hits) != 1 || !strings.Contains(hits[0], shell.PreviousVar) {
		t.Errorf("collisions = %q, want previous_dir -> %s", hits, shell.PreviousVar)
	}
}
