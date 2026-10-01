package cli_test

import (
	"testing"

	"github.com/jMautone/worktree-manager/internal/cli"
)

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		injected, module, want string
	}{
		// Release builds inject the version; it wins over the module's.
		{"0.1.0-alpha.1", "v0.1.0-alpha.1", "0.1.0-alpha.1"},
		{"9.9.9-e2e", "(devel)", "9.9.9-e2e"},
		// go install …@v0.1.0-alpha.1 records only the module version.
		{"", "v0.1.0-alpha.1", "0.1.0-alpha.1"},
		{"", "v1.0.0", "1.0.0"},
		// A go build in a checkout records a pseudo-version.
		{"", "v0.1.0-alpha.2.0.20261001120000-a26353022941+dirty", "0.1.0-alpha.2.0.20261001120000-a26353022941+dirty"},
		// Nothing recorded.
		{"", "(devel)", cli.DevVersion},
		{"", "", cli.DevVersion},
	}
	for _, c := range cases {
		if got := cli.ResolveVersion(c.injected, c.module); got != c.want {
			t.Errorf("ResolveVersion(%q, %q) = %q, want %q", c.injected, c.module, got, c.want)
		}
	}
}
