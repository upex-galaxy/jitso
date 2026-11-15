package fail

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeAndText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		text string
	}{
		{"nil", nil, 0, ""},
		{"rejected", Rejected(`unknown build "x"`), 2, `JITSO REJECTED: unknown build "x"`},
		{"rejectedf", Rejectedf("missing %s", "ticket_key"), 2, "JITSO REJECTED: missing ticket_key"},
		{"usage", Usage("jitso: unknown command \"x\"\n\nUsage"), 2, "jitso: unknown command \"x\"\n\nUsage"},
		{"env", Env("the lock stayed busy"), 3, "jitso: the lock stayed busy"},
		{"envf", Envf("%s is not inside a git checkout", "/tmp/x"), 3, "jitso: /tmp/x is not inside a git checkout"},
		{"status", Status(7), 7, ""},
		{"status zero", Status(0), 0, ""},
		{"plain error", errors.New("boom"), 3, "jitso: boom"},
		{"wrapped rejected", fmt.Errorf("ctx: %w", Rejected("bad")), 2, "JITSO REJECTED: bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.err); got != tt.code {
				t.Errorf("Code = %d, want %d", got, tt.code)
			}
			if got := Text(tt.err); got != tt.text {
				t.Errorf("Text = %q, want %q", got, tt.text)
			}
		})
	}
}

func TestRejectedText(t *testing.T) {
	if got := RejectedText("x"); got != "JITSO REJECTED: x" {
		t.Fatalf("got %q", got)
	}
}
