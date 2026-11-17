package engine

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		held, runnerError bool
		want              Status
	}{
		{true, false, Pass},
		{false, false, Fail},
		{true, true, Fail},
		{false, true, Fail},
	}
	for _, c := range cases {
		if got := Classify(c.held, c.runnerError); got != c.want {
			t.Errorf("Classify(held=%v, runnerError=%v) = %s, want %s", c.held, c.runnerError, got, c.want)
		}
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		statuses   []Status
		loadErrors int
		want       int
	}{
		{[]Status{Pass, Skip}, 0, 0},
		{[]Status{Fail}, 0, 1},
		{[]Status{Pass}, 1, 2},
		{[]Status{Fail}, 1, 2}, // a load error wins
		{nil, 0, 0},
	}
	for _, c := range cases {
		if got := ExitCode(c.statuses, c.loadErrors); got != c.want {
			t.Errorf("ExitCode(%v, %d) = %d, want %d", c.statuses, c.loadErrors, got, c.want)
		}
	}
}

func TestParseDecision(t *testing.T) {
	cases := []struct{ stdout, decision, reason string }{
		{"", "none", ""},
		{"\n", "none", ""},
		{`{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"r"}}` + "\n", "deny", "r"},
		{`{"hookSpecificOutput":{"permissionDecision":"allow"}}`, "allow", ""},
		{"garbage", "(stdout is not one JSON object)", ""},
		{`{"other":1}`, "(no hookSpecificOutput.permissionDecision)", ""},
	}
	for _, c := range cases {
		d, r := ParseDecision(c.stdout)
		if d != c.decision || r != c.reason {
			t.Errorf("ParseDecision(%q) = %q, %q; want %q, %q", c.stdout, d, r, c.decision, c.reason)
		}
	}
}
