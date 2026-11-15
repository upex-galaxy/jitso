package cli

import (
	"strings"
	"testing"
)

func TestHook(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{"unknown name", []string{"hook", "nope"}, 2, "", `jitso hook: unknown hook "nope"`},
		{"too many words", []string{"hook", "guard", "x"}, 2, "", "jitso hook: expected one of"},
		{"guard fails open outside a project", []string{"hook", "guard"}, 0, "", ""},
		{"dispatch-guard fails closed", []string{"hook", "dispatch-guard"}, 0, `"permissionDecision":"deny"`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(t, tt.args...)
			if code != tt.code || !strings.Contains(out, tt.stdout) || (tt.stdout == "" && out != "") || !strings.Contains(errOut, tt.stderr) || (tt.stderr == "" && errOut != "") {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
}
