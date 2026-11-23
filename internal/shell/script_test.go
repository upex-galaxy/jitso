package shell

import (
	"runtime"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
)

func TestRunScript(t *testing.T) {
	tests := []struct {
		name    string
		env     []harness.EnvVar
		argv    []string
		want    string
		wantErr bool
	}{
		{"plain", nil, []string{"claude", "--model", "m"}, "#!/bin/sh\nexec 'claude' '--model' 'm'\n", false},
		{"env lines before exec", []harness.EnvVar{{Name: "CLAUDE_CODE_DISABLE_CLAUDE_MDS", Value: "1"}, {Name: "_X", Value: "it's"}},
			[]string{"claude"}, "#!/bin/sh\nexport CLAUDE_CODE_DISABLE_CLAUDE_MDS='1'\nexport _X='it'\\''s'\nexec 'claude'\n", false},
		{"hostile word stays one word", nil, []string{"claude", "x; touch canary\n$(y)"}, "#!/bin/sh\nexec 'claude' 'x; touch canary\n$(y)'\n", false},
		{"empty argv", nil, nil, "", true},
		{"program with a slash", nil, []string{"/bin/claude"}, "", true},
		{"program upper case", nil, []string{"Claude"}, "", true},
		{"env name lower case", []harness.EnvVar{{Name: "lower", Value: "1"}}, []string{"claude"}, "", true},
		{"env name with a space", []harness.EnvVar{{Name: "A B", Value: "1"}}, []string{"claude"}, "", true},
		{"env name starting with a digit", []harness.EnvVar{{Name: "1A", Value: "1"}}, []string{"claude"}, "", true},
		{"NUL in a word", nil, []string{"claude", "a\x00b"}, "", true},
		{"NUL in a value", []harness.EnvVar{{Name: "A", Value: "\x00"}}, []string{"claude"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RunScript(tt.env, tt.argv)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandFor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("jitso launch does not run on Windows; a /p path is not absolute there")
	}
	if got, err := CommandFor("/p/.jitso/launches/L1/run.sh"); err != nil || got != "'/p/.jitso/launches/L1/run.sh'" {
		t.Errorf("got %q, %v", got, err)
	}
	if got, err := CommandFor("/a b/it's/run.sh"); err != nil || got != `'/a b/it'\''s/run.sh'` {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := CommandFor("relative/run.sh"); err == nil {
		t.Error("a relative path must fail")
	}
}

func TestTerminalTitle(t *testing.T) {
	if got, err := TerminalTitle("L20261113T142233-a3f9"); err != nil || got != "[jitso:L20261113T142233-a3f9]" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "L20261113T142233-A3F9", "x] evil", "L20261113T142233-a3f9\n"} {
		if _, err := TerminalTitle(bad); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
}
