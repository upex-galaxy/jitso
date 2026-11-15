package cli

import "testing"

func TestRequestRejections(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		stderr string
	}{
		{"starts with a dash", []string{"-x y"}, "JITSO REJECTED: the request must not start with a dash"},
		{"blank", []string{"   "}, "JITSO REJECTED: the request is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(t, tt.args...)
			if code != 2 || out != "" || errOut != tt.stderr+"\n" {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
}
