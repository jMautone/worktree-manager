package config

import "testing"

func TestUserFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		goos string
		env  map[string]string
		home string
		want string
	}{
		{"macOS default", "darwin", nil, "/Users/me", "/Users/me/.config/wt/config.toml"},
		{"linux default", "linux", nil, "/home/me", "/home/me/.config/wt/config.toml"},
		{"macOS XDG", "darwin", map[string]string{"XDG_CONFIG_HOME": "/tmp/cfg"}, "/Users/me", "/tmp/cfg/wt/config.toml"},
		{"linux XDG", "linux", map[string]string{"XDG_CONFIG_HOME": "/tmp/cfg"}, "/home/me", "/tmp/cfg/wt/config.toml"},
		{"relative XDG is ignored", "linux", map[string]string{"XDG_CONFIG_HOME": "cfg"}, "/home/me", "/home/me/.config/wt/config.toml"},
		{"empty XDG is ignored", "darwin", map[string]string{"XDG_CONFIG_HOME": ""}, "/Users/me", "/Users/me/.config/wt/config.toml"},
		{"windows APPDATA", "windows", map[string]string{"APPDATA": `C:\Users\me\AppData\Roaming`}, `C:\Users\me`, `C:\Users\me\AppData\Roaming\wt\config.toml`},
		{"windows APPDATA with trailing separator", "windows", map[string]string{"APPDATA": `C:\Users\me\AppData\Roaming\`}, `C:\Users\me`, `C:\Users\me\AppData\Roaming\wt\config.toml`},
		{"windows ignores XDG", "windows", map[string]string{"APPDATA": `C:\A`, "XDG_CONFIG_HOME": "/tmp/cfg"}, `C:\Users\me`, `C:\A\wt\config.toml`},
		{"WT_CONFIG on macOS", "darwin", map[string]string{"WT_CONFIG": "/some/where/wt.toml", "XDG_CONFIG_HOME": "/tmp/cfg"}, "/Users/me", "/some/where/wt.toml"},
		{"WT_CONFIG on linux", "linux", map[string]string{"WT_CONFIG": "/some/where/wt.toml"}, "/home/me", "/some/where/wt.toml"},
		{"WT_CONFIG on windows", "windows", map[string]string{"WT_CONFIG": `D:\cfg\wt.toml`, "APPDATA": `C:\A`}, `C:\Users\me`, `D:\cfg\wt.toml`},
		{"empty WT_CONFIG is ignored", "darwin", map[string]string{"WT_CONFIG": ""}, "/Users/me", "/Users/me/.config/wt/config.toml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			got, err := UserFile(tc.goos, getenv, tc.home)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("UserFile = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUserFileWithoutBaseDirectory(t *testing.T) {
	getenv := func(string) string { return "" }
	if _, err := UserFile("darwin", getenv, ""); err == nil {
		t.Error("macOS without HOME or XDG_CONFIG_HOME: no error")
	}
	if _, err := UserFile("windows", getenv, `C:\Users\me`); err == nil {
		t.Error("windows without APPDATA: no error")
	}
}
