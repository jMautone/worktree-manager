package cli

import (
	"errors"
	"fmt"
	"testing"
)

// The exit codes are public contract (cli-contract spec, "Exit codes").
func TestExitCodesMatchContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"ExitOK", ExitOK, 0},
		{"ExitError", ExitError, 1},
		{"ExitUsage", ExitUsage, 2},
		{"ExitNotFound", ExitNotFound, 3},
		{"ExitAmbiguous", ExitAmbiguous, 4},
		{"ExitBlocked", ExitBlocked, 5},
		{"ExitConflict", ExitConflict, 6},
		{"ExitHookFailed", ExitHookFailed, 7},
		{"ExitHookNotApproved", ExitHookNotApproved, 8},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestErrorCarriesCodeMessageAndHints(t *testing.T) {
	var err error = &Error{Code: ExitNotFound, Msg: "not a git repository: /x", Hints: []string{"run it inside a repository"}}

	if err.Error() != "not a git repository: /x" {
		t.Errorf("Error() = %q", err.Error())
	}
	var e *Error
	if !errors.As(fmt.Errorf("wrapped: %w", err), &e) || e.Code != ExitNotFound || len(e.Hints) != 1 {
		t.Errorf("errors.As through a wrap = %+v", e)
	}
}
