package main

import "testing"

const sampleChangelog = `# Changelog

## [Unreleased]

### Added

- Something new.

## [0.1.0] — 2026-10-15

### Added

- ` + "`wt list`" + `.

### Fixed

- A bug.

---

## [0.1.01] — not a real version

## PowerShell 0.9.0 — 2026-09-21

Last PowerShell release.
`

func TestChangelogSection(t *testing.T) {
	body, ok := ChangelogSection(sampleChangelog, Version{Minor: 1})
	want := "### Added\n\n- `wt list`.\n\n### Fixed\n\n- A bug."
	if !ok || body != want {
		t.Errorf("ChangelogSection(0.1.0) = %q, %v; want %q", body, ok, want)
	}

	// CRLF checkouts (Windows without .gitattributes) read the same.
	crlf := ""
	for _, r := range sampleChangelog {
		if r == '\n' {
			crlf += "\r\n"
		} else {
			crlf += string(r)
		}
	}
	if body, ok := ChangelogSection(crlf, Version{Minor: 1}); !ok || body != want {
		t.Errorf("CRLF: got %q, %v", body, ok)
	}

	for _, v := range []Version{{Minor: 2}, {Minor: 1, Alpha: 1}, {Minor: 9}} {
		if _, ok := ChangelogSection(sampleChangelog, v); ok {
			t.Errorf("ChangelogSection(%s) found a section that does not exist", v)
		}
	}
}

func TestChangelogSectionIgnoresLookAlikeHeadings(t *testing.T) {
	lookAlikes := "# Changelog\n\n## [0.1.01] — not 0.1.0\n\n- wrong.\n\n## [0.1.0-alpha.1]\n\n- wrong too.\n"
	if body, ok := ChangelogSection(lookAlikes, Version{Minor: 1}); ok {
		t.Errorf("ChangelogSection(v0.1.0) matched a look-alike heading: %q", body)
	}

	// A look-alike before the real section must not shadow it.
	both := lookAlikes + "\n## [0.1.0] — 2026-10-15\n\n- right.\n"
	if body, ok := ChangelogSection(both, Version{Minor: 1}); !ok || body != "- right." {
		t.Errorf("ChangelogSection(v0.1.0) = %q, %v; want %q", body, ok, "- right.")
	}
}
