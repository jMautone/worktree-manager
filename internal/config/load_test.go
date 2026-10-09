package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	layer, err := Load(filepath.Join(t.TempDir(), "nope.toml"), SourceUser)
	if err != nil || layer != nil {
		t.Errorf("Load of a missing file = %v, %v; want nil, nil", layer, err)
	}
}

func TestLoadDecodesValues(t *testing.T) {
	p := writeFile(t, "# comment\ndefault_base = \"origin/main\"\ncolour = \"blue\"\n")
	layer, err := Load(p, SourceUser)
	if err != nil {
		t.Fatal(err)
	}
	if layer.Source != SourceUser || layer.Origin != p {
		t.Errorf("layer source/origin = %s, %q", layer.Source, layer.Origin)
	}
	if layer.Values["default_base"] != "origin/main" || layer.Values["colour"] != "blue" {
		t.Errorf("values = %#v", layer.Values)
	}
}

func TestLoadKeepsTypesForValidation(t *testing.T) {
	p := writeFile(t, "default_base = 42\n")
	layer, err := Load(p, SourceUser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(Keys("darwin"), *layer); err == nil || !strings.Contains(err.Error(), "integer") {
		t.Errorf("Resolve of an integer default_base = %v, want a type error", err)
	}
}

func TestLoadMalformedTOMLNamesFileAndLine(t *testing.T) {
	p := writeFile(t, "# wt config\n\ndefault_base = \n")
	_, err := Load(p, SourceUser)
	if err == nil {
		t.Fatal("no error for malformed TOML")
	}
	if !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error %q does not name the file and line 3", err)
	}
}

func TestLoadUnreadableFileFails(t *testing.T) {
	// A directory where the file should be: exists, but cannot be read.
	dir := t.TempDir()
	if _, err := Load(dir, SourceUser); err == nil {
		t.Error("no error reading a directory as a config file")
	}
}
