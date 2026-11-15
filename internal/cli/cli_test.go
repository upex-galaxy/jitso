package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, System{
		Stdout:    &out,
		Stderr:    &errOut,
		LookupEnv: func(string) (string, bool) { return "", false },
		Getwd:     func() (string, error) { return "", errors.New("the cwd must not be read") },
	})
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		code           int
		stdout, stderr string
	}{
		{"no arguments", nil, 0, "Usage: jitso", ""},
		{"--help", []string{"--help"}, 0, "Usage: jitso", ""},
		{"-h", []string{"-h", "extra"}, 0, "Usage: jitso", ""},
		{"unknown", []string{"frobnicate"}, 2, "", `jitso: unknown command "frobnicate"`},
		{"case matters", []string{"Launch"}, 2, "", `unknown command "Launch"`},
		{"empty word", []string{""}, 2, "", `jitso: unknown command ""`},
		{"dispatch is not a word", []string{"dispatch"}, 2, "", `unknown command "dispatch"`},
		{"tree", []string{"tree"}, 0, "jitso tree: not implemented yet\n", ""},
		{"tree with root", []string{"tree", "x"}, 0, "jitso tree: not implemented yet\n", ""},
		{"presets is not a command", []string{"presets"}, 2, "", `jitso: unknown command "presets"`},
		{"presets list is not a command", []string{"presets", "list"}, 2, "", `jitso: unknown command "presets"`},
		{"build alone", []string{"build"}, 2, "", "jitso build: missing subcommand\n\nUsage: jitso build <subcommand>"},
		{"build unknown subcommand", []string{"build", "show"}, 2, "", "jitso build: unknown subcommand \"show\"\n\nUsage: jitso build <subcommand>"},
		{"build --help", []string{"build", "--help"}, 0, "Usage: jitso build <subcommand>", ""},
		{"build -h", []string{"build", "-h"}, 0, "Usage: jitso build <subcommand>", ""},
		{"build list needs the project", []string{"build", "list"}, 3, "", "jitso: the cwd must not be read"},
		{"request needs the project", []string{"test PRJ-1", "now"}, 3, "", "jitso: the cwd must not be read"},
		{"launch checks its arguments before the project", []string{"launch"}, 2, "", "JITSO REJECTED: --build is required"},
		{"hook needs a name", []string{"hook"}, 2, "", "jitso hook: expected one of guard or dispatch-guard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(t, tt.args...)
			if code != tt.code {
				t.Errorf("exit %d, want %d (stderr %q)", code, tt.code, errOut)
			}
			if tt.stdout == "" && out != "" || !strings.Contains(out, tt.stdout) {
				t.Errorf("stdout %q, want %q", out, tt.stdout)
			}
			if tt.stderr == "" && errOut != "" || !strings.Contains(errOut, tt.stderr) {
				t.Errorf("stderr %q, want %q", errOut, tt.stderr)
			}
		})
	}
}

func TestUnknownCommandPrintsUsage(t *testing.T) {
	_, _, errOut := run(t, "x")
	if !strings.HasPrefix(errOut, "jitso: unknown command \"x\"\n\nUsage: jitso") || !strings.HasSuffix(errOut, "harness error.\n") {
		t.Errorf("stderr %q", errOut)
	}
}

func TestUsageOrder(t *testing.T) {
	u := Usage()
	last := -1
	for _, name := range []string{"launch", "close", "reap", "tree", "analyze", "build", "hook"} {
		i := strings.Index(u, "\n  "+name+" ")
		if i <= last {
			t.Fatalf("%s out of order in\n%s", name, u)
		}
		last = i
	}
	if !strings.Contains(u, "--help") {
		t.Error("usage must name --help")
	}
}

func TestIsRequest(t *testing.T) {
	for word, want := range map[string]bool{
		"launch":     false,
		"":           false,
		"a b":        true,
		"a\tb":       true,
		"a\u00a0b":   true,
		"a\u2003b":   true,
		"a\ufeffb":   true,
		"a\u0085b":   false,
		"a\u200bb":   false,
		"trailing\n": true,
	} {
		if IsRequest(word) != want {
			t.Errorf("IsRequest(%q) != %v", word, want)
		}
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("want a panic")
		}
	}()
	Register(Command{Name: "tree", Run: func(*Context, []string) error { return nil }})
}
