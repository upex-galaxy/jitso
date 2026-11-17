package shell

import (
	"errors"
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "''"},
		{"plain", "'plain'"},
		{"it's", `'it'\''s'`},
		{"''", `''\'''\'''`},
		{"$(rm -rf /) `x` \"y\" ; | & \n", "'$(rm -rf /) `x` \"y\" ; | & \n'"},
		{"ñ 日本", "'ñ 日本'"},
	}
	for _, tt := range tests {
		got, err := Quote(tt.in)
		if err != nil {
			t.Fatalf("Quote(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("Quote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if _, err := Quote("a\x00b"); !errors.Is(err, ErrNUL) {
		t.Errorf("NUL: got %v, want ErrNUL", err)
	}
}

func TestCheckLaunchField(t *testing.T) {
	long := func(n int) string { return "a" + strings.Repeat("b", n-1) }
	tests := []struct {
		name, field, value string
		ok                 bool
	}{
		{"build ok", "build", "haiku-explorer", true},
		{"build upper", "build", "Haiku", false},
		{"build dash first", "build", "-x", false},
		{"build 64", "build", long(64), true},
		{"build 65", "build", long(65), false},
		{"build dots", "build", "a..b", false},
		{"run ok", "run", "run_1.a:b-c", true},
		{"run upper first", "run", "R1", true},
		{"run dash first", "run", "-r", false},
		{"run space", "run", "r 1", false},
		{"run long s", "run", "r\u017f", false},
		{"run kelvin", "run", "\u212a1", false},
		{"task ok", "task", "task-1", true},
		{"task slash", "task", "a/b", false},
		{"parent ok", "parent", "L20261113T142233-ab12", true},
		{"parent upper hex", "parent", "L20261113T142233-AB12", false},
		{"parent short", "parent", "L2026111T142233-ab12", false},
		{"worktree ok", "worktree", "wt/feature-x", true},
		{"worktree abs", "worktree", "/abs", false},
		{"worktree dots", "worktree", "a/../b", false},
		{"worktree 512", "worktree", long(512), true},
		{"worktree 513", "worktree", long(513), false},
		{"worktree kelvin", "worktree", "a\u212a", false},
		{"mcp ok", "mcp-exclude", "dbhub", true},
		{"mcp dot", "mcp-exclude", "db.hub", false},
		{"newline", "build", "a\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := CheckLaunchField(tt.field, tt.value)
			if (reason == "") != tt.ok {
				t.Fatalf("CheckLaunchField(%q, %q) = %q, want ok=%v", tt.field, tt.value, reason, tt.ok)
			}
			if !tt.ok && reason != "invalid value for --"+tt.field {
				t.Errorf("reason = %q", reason)
			}
		})
	}
	if got := CheckLaunchField("brief", "x"); got != `unknown launcher field "brief"` {
		t.Errorf("unknown field: %q", got)
	}
}
