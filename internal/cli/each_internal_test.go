package cli

import (
	"errors"
	"reflect"
	"testing"
)

func TestEachReport(t *testing.T) {
	ok := func(label string) eachResult { return eachResult{Label: label} }
	for _, tc := range []struct {
		name        string
		results     []eachResult
		planned     int
		interrupted bool
		want        *Error
	}{
		{"every command exits with 0", []eachResult{ok("^"), ok("feat"), ok("fix")}, 3, false, nil},
		{"no worktree", nil, 0, false, nil},
		{
			"one exits with 3",
			[]eachResult{ok("^"), {Label: "feat", Code: 3}, ok("fix")}, 3, false,
			&Error{Code: ExitError, Msg: "1 of 3 commands failed", Hints: []string{"feat exited with 3"}},
		},
		{
			"one could not start",
			[]eachResult{{Label: "api:^", Err: errors.New("no such directory")}, {Label: "api:feat", Code: 143}}, 2, false,
			&Error{Code: ExitError, Msg: "2 of 2 commands failed", Hints: []string{"api:^ could not start: no such directory", "api:feat exited with 143"}},
		},
		{
			"interrupted after the first of two",
			[]eachResult{{Label: "^", Code: 3}}, 2, true,
			&Error{Code: ExitInterrupted, Msg: "interrupted: 1 of 2 commands did not run", Hints: []string{"^ exited with 3"}},
		},
		{
			"interrupted during the last",
			[]eachResult{ok("^"), ok("feat")}, 2, true,
			&Error{Code: ExitInterrupted, Msg: "interrupted: 0 of 2 commands did not run"},
		},
		{
			"interrupted before the first",
			nil, 2, true,
			&Error{Code: ExitInterrupted, Msg: "interrupted: 2 of 2 commands did not run"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := eachReport(tc.results, tc.planned, tc.interrupted)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("got %v, want nil", err)
				}
				return
			}
			var e *Error
			if !errors.As(err, &e) || !reflect.DeepEqual(e, tc.want) {
				t.Errorf("got %#v, want %#v", err, tc.want)
			}
		})
	}
}
