package template

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	values := map[string]string{
		"repo":        "repo",
		"repo_parent": "/src",
		"repo_path":   "/src/repo",
		"name":        "Feat",
		"branch":      "Feature/ABC",
	}
	for _, tc := range []struct {
		src, want string
	}{
		{"{repo_parent}/{repo}.worktrees/{name|sanitize}", "/src/repo.worktrees/Feat"},
		{"{repo_parent}/{branch|sanitize|lower}", "/src/feature-abc"},
		// Filters apply left to right: lower first keeps the slash for
		// sanitize to replace, with the same result here.
		{"{branch|lower|sanitize}", "feature-abc"},
		{"{repo_parent}/{ name | sanitize }", "/src/Feat"},
		{"{ repo_path }", "/src/repo"},
		{"{branch}", "Feature/ABC"},
		{"literal only", "literal only"},
		{"", ""},
		{"{repo}{name}", "repoFeat"},
		{"~/wt/{repo}/{name|lower}", "~/wt/repo/feat"},
	} {
		tmpl, err := Parse(tc.src, WorktreePathVars)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.src, err)
			continue
		}
		if got := tmpl.Render(values); got != tc.want {
			t.Errorf("Render(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestWhitespaceRendersLikeWithout(t *testing.T) {
	values := map[string]string{"repo_parent": "/src", "name": "feature/x"}
	a, err1 := Parse("{repo_parent}/{ name | sanitize }", WorktreePathVars)
	b, err2 := Parse("{repo_parent}/{name|sanitize}", WorktreePathVars)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if a.Render(values) != b.Render(values) {
		t.Errorf("%q != %q", a.Render(values), b.Render(values))
	}
}

func TestParseErrorsNameTheOffendingPart(t *testing.T) {
	for _, tc := range []struct {
		name, src, part string
	}{
		{"unknown variable", "{repo_parent}/{nope}", "nope"},
		{"unknown variable with filter", "{ nope |sanitize}", "nope"},
		{"unknown filter", "{repo_parent}/{name|upper}", "upper"},
		{"unknown filter after a known one", "{name|sanitize|hash_port}", "hash_port"},
		{"unclosed brace", "{repo_parent/x", "{repo_parent/x"},
		{"unclosed brace before another", "{repo_parent/{name}", "{repo_parent/"},
		{"stray closing brace", "{repo}/x}", "/x}"},
		{"stray closing brace at the start", "}x", "}"},
		{"empty expression", "{repo}/{}", "{}"},
		{"blank expression", "{repo}/{ }", "{ }"},
		{"filter without a variable", "{|sanitize}", "{|sanitize}"},
		{"empty filter", "{name|}", "{name|}"},
		{"empty filter in the middle", "{name||lower}", "{name||lower}"},
		{"blank filter", "{name| |lower}", "{name| |lower}"},
		{"variable not offered", "{path}", "path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, WorktreePathVars)
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("Parse(%q) = %v, want a *SyntaxError", tc.src, err)
			}
			if se.Part != tc.part {
				t.Errorf("Part = %q, want %q", se.Part, tc.part)
			}
			if !strings.Contains(err.Error(), strconv.Quote(tc.part)) {
				t.Errorf("message %q does not name %q", err, tc.part)
			}
		})
	}
}

func TestParseOnlyAcceptsTheGivenVariables(t *testing.T) {
	if _, err := Parse("{name}", []string{"branch"}); err == nil {
		t.Error("{name} parsed with only branch available")
	}
	if _, err := Parse("{branch}", []string{"branch"}); err != nil {
		t.Error(err)
	}
}

// The table of the spec "sanitize filter", and the edges of each rule.
func TestSanitize(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"feature/abc1", "feature-abc1"},
		{`fix\x`, "fix-x"},
		{"a:b*c", "a-b-c"},
		{"feat<x>", "feat-x-"},
		{"v1.", "v1-"},
		{"..", "--"},
		{"nul", "nul-"},
		{"Con.txt", "Con-.txt"},
		{"my task", "my task"},
		{"café", "café"},

		{`a"b|c?d`, "a-b-c-d"},
		{"tab\there", "tab-here"},
		{"nl\nx\x00y\x1fz\x7f", "nl-x-y-z-"},
		{"x. .", "x---"},
		{"trailing space ", "trailing space-"},
		{" leading", " leading"},
		{".hidden", ".hidden"},
		{"COM1", "COM1-"},
		{"lpt9.txt", "lpt9-.txt"},
		{"LpT1.tar.gz", "LpT1-.tar.gz"},
		{"console", "console"},
		{"com10", "com10"},
		{"nulx", "nulx"},
		// Rule 2 before rule 3: the trailing dot is replaced first, so
		// what is left is not a reserved name.
		{"con.", "con-"},
		{"aux/x", "aux-x"},
		{"", ""},
		{"日本/語", "日本-語"},
	} {
		if got := Sanitize(tc.in); got != tc.want {
			t.Errorf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLower(t *testing.T) {
	tmpl, err := Parse("{name|lower}", WorktreePathVars)
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{"Feat": "feat", "ÉCOLE/X-1": "école/x-1", "ΣΑ": "σα"} {
		if got := tmpl.Render(map[string]string{"name": in}); got != want {
			t.Errorf("lower(%q) = %q, want %q", in, got, want)
		}
	}
}

// Sanitize takes only the text, and the package reads nothing of the OS it
// runs on: the same branch must give the same directory on every OS.
func TestSanitizeDoesNotDependOnTheOS(t *testing.T) {
	var _ func(string) string = Sanitize

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			switch path, _ := strconv.Unquote(imp.Path.Value); path {
			case "os", "runtime", "path/filepath", "syscall":
				t.Errorf("%s imports %s", f, path)
			}
		}
		if parsed.Name.Name != "template" || strings.Contains(f, "_windows") || strings.Contains(f, "_darwin") || strings.Contains(f, "_unix") {
			t.Errorf("%s is built per OS", f)
		}
	}
}
