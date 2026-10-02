package cli_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

const defaultWorktreePath = "{repo_parent}/{repo}.worktrees/{name|sanitize}"

// configLine returns the line of `wt config list` for key.
func configLine(t *testing.T, stdout, key string) []string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == key {
			return f
		}
	}
	t.Fatalf("no line for %s in\n%s", key, stdout)
	return nil
}

func TestConfigPathOutsideRepository(t *testing.T) {
	h := newHarness(t)

	r := h.run("config", "path")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, h.userConfigPath()) {
		t.Errorf("stdout does not show the user file %s:\n%s", h.userConfigPath(), r.stdout)
	}
	if !strings.Contains(r.stdout, "repo: none") {
		t.Errorf("stdout does not report the repository file as none:\n%s", r.stdout)
	}

	r = h.run("config", "path", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	user, _ := doc["user"].(map[string]any)
	if doc["schema"] != "wt.config.path.v1" || user["path"] != h.userConfigPath() || user["exists"] != false {
		t.Errorf("document = %v", doc)
	}
	if repo, ok := doc["repo"]; !ok || repo != nil {
		t.Errorf("repo = %v, want null", doc["repo"])
	}
}

func TestConfigPathInsideRepository(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "")
	h.userConfig("")
	h.cwd = filepath.Join(repo, "sub")
	h.sb.WriteFile(filepath.Join(h.cwd, "f"), "")

	r := h.run("config", "path", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	user, _ := doc["user"].(map[string]any)
	repoFile, _ := doc["repo"].(map[string]any)
	if user["exists"] != true {
		t.Errorf("user = %v, want exists", user)
	}
	want := filepath.Join(testutil.Comparable(t, repo), ".wt.toml")
	if repoFile["path"] != want || repoFile["exists"] != true {
		t.Errorf("repo = %v, want path %s and exists", repoFile, want)
	}
}

// A bare repository has no working tree, so no repository layer, even with
// a .wt.toml in its directory.
func TestConfigPathInsideBareRepository(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("bare.git")
	h.sb.InitBare(bare)
	h.sb.WriteFile(filepath.Join(bare, ".wt.toml"), "default_base = \"origin/bare\"\n")
	h.cwd = bare

	r := h.run("config", "path")
	r.mustCode(t, 0)
	if !strings.Contains(r.stdout, "repo: none") {
		t.Errorf("stdout = %q, want no repository file", r.stdout)
	}
	if r := h.run("config", "get", "default_base"); r.stdout != "\n" {
		t.Errorf("default_base = %q, want the default", r.stdout)
	}
}

func TestConfigPathDefaultLocationOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the default user file on Windows is under %APPDATA%")
	}
	h := newHarness(t)
	h.sb.Setenv("XDG_CONFIG_HOME", "")

	doc := decodeOne(t, h.run("config", "path", "--json").stdout)
	user, _ := doc["user"].(map[string]any)
	if want := filepath.Join(h.sb.Getenv("HOME"), ".config", "wt", "config.toml"); user["path"] != want {
		t.Errorf("user file = %v, want %s", user["path"], want)
	}
}

func TestConfigPathExplicitOverride(t *testing.T) {
	h := newHarness(t)
	custom := h.sb.Path("custom", "wt.toml")
	h.sb.Setenv("WT_CONFIG", custom)
	h.sb.WriteFile(custom, "default_base = \"origin/custom\"\n")

	doc := decodeOne(t, h.run("config", "path", "--json").stdout)
	if user, _ := doc["user"].(map[string]any); user["path"] != custom || user["exists"] != true {
		t.Errorf("user = %v, want %s", user, custom)
	}
	if r := h.run("config", "get", "default_base"); r.stdout != "origin/custom\n" {
		t.Errorf("default_base = %q, want the value from WT_CONFIG's file", r.stdout)
	}
}

func TestConfigListSources(t *testing.T) {
	h := newHarness(t)
	h.userConfig("default_base = \"origin/main\"\n")

	r := h.run("config", "list")
	r.mustCode(t, 0)
	if f := configLine(t, r.stdout, "default_base"); len(f) != 3 || f[1] != "origin/main" || f[2] != "user" {
		t.Errorf("default_base line = %q", f)
	}
	if f := configLine(t, r.stdout, "worktree_path"); len(f) != 3 || f[1] != defaultWorktreePath || f[2] != "default" {
		t.Errorf("worktree_path line = %q", f)
	}

	r = h.run("config", "list", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	keys, _ := doc["keys"].([]any)
	if doc["schema"] != "wt.config.list.v1" || len(keys) != 5 {
		t.Fatalf("document = %v", doc)
	}
	first, _ := keys[0].(map[string]any)
	second, _ := keys[1].(map[string]any)
	if first["key"] != "default_base" || first["value"] != "origin/main" || first["source"] != "user" {
		t.Errorf("keys[0] = %v", first)
	}
	if second["key"] != "worktree_path" || second["value"] != defaultWorktreePath || second["source"] != "default" {
		t.Errorf("keys[1] = %v", second)
	}
}

func TestConfigListShowsEmptyValue(t *testing.T) {
	r := newHarness(t).run("config", "list")
	r.mustCode(t, 0)
	if f := configLine(t, r.stdout, "default_base"); len(f) != 3 || f[1] != `""` || f[2] != "default" {
		t.Errorf("default_base line = %q, want an explicit empty value", f)
	}
}

func TestConfigGet(t *testing.T) {
	h := newHarness(t)

	r := h.run("config", "get", "worktree_path")
	r.mustCode(t, 0)
	if r.stdout != defaultWorktreePath+"\n" {
		t.Errorf("stdout = %q, want only the value", r.stdout)
	}

	r = h.run("config", "get", "worktree_path", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.config.get.v1" || doc["key"] != "worktree_path" ||
		doc["value"] != defaultWorktreePath || doc["source"] != "default" {
		t.Errorf("document = %v", doc)
	}
}

func TestConfigGetUnknownKey(t *testing.T) {
	r := newHarness(t).run("config", "get", "nope")
	r.mustCode(t, 2)
	if r.stdout != "" || !strings.Contains(r.stderr, `"nope"`) {
		t.Errorf("stdout %q, stderr %q; want an error naming nope", r.stdout, r.stderr)
	}
}

func TestConfigWrongType(t *testing.T) {
	h := newHarness(t)
	file := h.userConfig("default_base = 42\n")

	r := h.run("config", "list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "default_base") || !strings.Contains(r.stderr, file) {
		t.Errorf("stderr %q does not name default_base and %s", r.stderr, file)
	}
}

func TestConfigUnknownKeyWarns(t *testing.T) {
	h := newHarness(t)
	h.userConfig("colour = \"blue\"\n")

	r := h.run("config", "list")
	r.mustCode(t, 0)
	if !strings.Contains(r.stderr, "wt: warning: ") || !strings.Contains(r.stderr, "colour") {
		t.Errorf("stderr = %q, want a warning naming colour", r.stderr)
	}
}

func TestConfigMalformedTOML(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	file := h.userConfig("# wt\n\ndefault_base = \n")

	r := h.run("list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, file) || !strings.Contains(r.stderr, "line 3") {
		t.Errorf("stderr %q does not name %s and line 3", r.stderr, file)
	}
}

func TestConfigRepositoryFile(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \"origin/develop\"\n")

	h.cwd = repo
	if r := h.run("config", "get", "default_base"); r.stdout != "origin/develop\n" {
		t.Errorf("inside the repository: %q, want origin/develop", r.stdout)
	}

	// The file of the worktree you are in, not the main one's.
	linked := h.sb.Path("linked")
	h.sb.AddWorktree(repo, linked, "feature/x")
	h.sb.WriteFile(filepath.Join(linked, ".wt.toml"), "default_base = \"origin/linked\"\n")
	h.cwd = linked
	if r := h.run("config", "get", "default_base"); r.stdout != "origin/linked\n" {
		t.Errorf("inside the linked worktree: %q, want origin/linked", r.stdout)
	}

	// Outside any worktree there is no repository layer.
	h.cwd = h.sb.Path("outside")
	if r := h.run("config", "get", "default_base"); r.stdout != "\n" {
		t.Errorf("outside: %q, want the default", r.stdout)
	}
}

// Scenarios of "Layer precedence", end to end.
func TestLayerPrecedence(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	h.cwd = repo
	h.userConfig("default_base = \"origin/main\"\n")
	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "default_base = \"origin/develop\"\n")

	get := func() (string, string) {
		t.Helper()
		r := h.run("config", "get", "default_base", "--json")
		r.mustCode(t, 0)
		doc := decodeOne(t, r.stdout)
		value, _ := doc["value"].(string)
		source, _ := doc["source"].(string)
		return value, source
	}

	h.sb.Setenv("WT_DEFAULT_BASE", "origin/release")
	if v, s := get(); v != "origin/release" || s != "env" {
		t.Errorf("environment beats files: got %s from %s", v, s)
	}

	h.sb.Setenv("WT_DEFAULT_BASE", "")
	if v, s := get(); v != "origin/develop" || s != "repo" {
		t.Errorf("repository beats user: got %s from %s", v, s)
	}

	h.sb.WriteFile(filepath.Join(repo, ".wt.toml"), "")
	if v, s := get(); v != "origin/main" || s != "user" {
		t.Errorf("empty environment variable is unset: got %s from %s", v, s)
	}
}

// Scenarios of "Configuration keys" and "Boolean values in the environment".
func TestConfigDefaultsOfCreate(t *testing.T) {
	h := newHarness(t)
	if r := h.run("config", "get", "worktree_path"); r.stdout != defaultWorktreePath+"\n" {
		t.Errorf("worktree_path = %q", r.stdout)
	}
	if r := h.run("config", "get", "create_cd"); r.stdout != "true\n" {
		t.Errorf("create_cd = %q, want true", r.stdout)
	}
	r := h.run("config", "list")
	r.mustCode(t, 0)
	for key, value := range map[string]string{"branch_prefix": `""`, "fetch_before_create": "true", "create_cd": "true"} {
		if f := configLine(t, r.stdout, key); len(f) != 3 || f[1] != value || f[2] != "default" {
			t.Errorf("%s line = %q", key, f)
		}
	}
}

func TestConfigStringForABooleanKey(t *testing.T) {
	h := newHarness(t)
	file := h.userConfig("create_cd = \"false\"\n")
	r := h.run("config", "list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "create_cd") || !strings.Contains(r.stderr, file) {
		t.Errorf("stderr %q does not name create_cd and %s", r.stderr, file)
	}
}

func TestConfigRepositorySetsCreateCd(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	h.sb.WriteFile(filepath.Join(h.cwd, ".wt.toml"), "create_cd = false\n")
	r := h.run("config", "get", "create_cd")
	r.mustCode(t, 0)
	if r.stdout != "true\n" {
		t.Errorf("create_cd = %q, want true", r.stdout)
	}
	if !strings.Contains(r.stderr, "wt: warning: ") || !strings.Contains(r.stderr, "create_cd") || !strings.Contains(r.stderr, ".wt.toml") {
		t.Errorf("stderr = %q, want a warning naming create_cd and .wt.toml", r.stderr)
	}
}

func TestConfigBooleansFromTheEnvironment(t *testing.T) {
	h := newHarness(t)

	h.sb.Setenv("WT_CREATE_CD", "0")
	if r := h.run("config", "get", "create_cd"); r.stdout != "false\n" {
		t.Errorf("WT_CREATE_CD=0: %q, want false", r.stdout)
	}
	if f := configLine(t, h.run("config", "list").stdout, "create_cd"); len(f) != 3 || f[1] != "false" || f[2] != "env" {
		t.Errorf("WT_CREATE_CD=0: list line %q", f)
	}
	h.sb.Setenv("WT_CREATE_CD", "")

	h.sb.Setenv("WT_FETCH_BEFORE_CREATE", "FALSE")
	if r := h.run("config", "get", "fetch_before_create"); r.stdout != "false\n" {
		t.Errorf("WT_FETCH_BEFORE_CREATE=FALSE: %q, want false", r.stdout)
	}
	h.sb.Setenv("WT_FETCH_BEFORE_CREATE", "")

	h.cwd = h.repo()
	h.sb.Setenv("WT_CREATE_CD", "yes")
	r := h.run("list")
	r.mustCode(t, 1)
	if !strings.Contains(r.stderr, "WT_CREATE_CD") {
		t.Errorf("WT_CREATE_CD=yes: stderr %q does not name the variable", r.stderr)
	}
}

func TestConfigBooleanAsJSON(t *testing.T) {
	r := newHarness(t).run("config", "get", "create_cd", "--json")
	r.mustCode(t, 0)
	if v := decodeOne(t, r.stdout)["value"]; v != true {
		t.Errorf("value = %#v, want the JSON boolean true", v)
	}
}

// Scenarios "Unknown variable", "Unknown filter" and "Unclosed brace" of
// path-templates: an invalid template is an invalid value.
func TestConfigInvalidTemplate(t *testing.T) {
	t.Run("unknown variable in the user file", func(t *testing.T) {
		h := newHarness(t)
		h.cwd = h.repo()
		file := h.userConfig("worktree_path = \"{repo_parent}/{nope}\"\n")
		r := h.run("list")
		r.mustCode(t, 1)
		for _, want := range []string{"worktree_path", file, "nope"} {
			if !strings.Contains(r.stderr, want) {
				t.Errorf("stderr %q does not name %q", r.stderr, want)
			}
		}
	})
	t.Run("unknown filter in .wt.toml", func(t *testing.T) {
		h := newHarness(t)
		h.cwd = h.repo()
		h.sb.WriteFile(filepath.Join(h.cwd, ".wt.toml"), "worktree_path = \"{repo_parent}/{name|upper}\"\n")
		r := h.run("config", "list")
		r.mustCode(t, 1)
		for _, want := range []string{"worktree_path", ".wt.toml", "upper"} {
			if !strings.Contains(r.stderr, want) {
				t.Errorf("stderr %q does not name %q", r.stderr, want)
			}
		}
	})
	t.Run("unclosed brace in the environment", func(t *testing.T) {
		h := newHarness(t)
		h.cwd = h.repo()
		h.sb.Setenv("WT_WORKTREE_PATH", "{repo_parent/x")
		h.activate()
		for _, args := range [][]string{{"list"}, {"config", "list"}, {"config", "get", "default_base"}, {"cd", "^"}, {"create", "feat"}} {
			r := h.run(args...)
			r.mustCode(t, 1)
			if !strings.Contains(r.stderr, "WT_WORKTREE_PATH") {
				t.Errorf("wt %v: stderr %q does not name WT_WORKTREE_PATH", args, r.stderr)
			}
		}
	})
}
