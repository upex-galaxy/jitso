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
		{"a request is not a command", []string{"test PRJ-1", "now"}, 2, "", `unknown command "test PRJ-1"`},
		{"tree needs the project", []string{"tree"}, 3, "", "jitso: the cwd must not be read"},
		{"tree with a launch id needs the project", []string{"tree", "L20261113T140000-0001"}, 3, "", "jitso: the cwd must not be read"},
		{"tree checks its arguments before the project", []string{"tree", "x"}, 2, "", "JITSO REJECTED: usage: jitso tree [<launch_id>]"},
		{"tree takes one id at most", []string{"tree", "L20261113T140000-0001", "L20261113T140000-0001"}, 2, "", "JITSO REJECTED: usage: jitso tree [<launch_id>]"},
		{"presets is not a command", []string{"presets"}, 2, "", `jitso: unknown command "presets"`},
		{"presets list is not a command", []string{"presets", "list"}, 2, "", `jitso: unknown command "presets"`},
		{"build alone", []string{"build"}, 2, "", "jitso build: missing subcommand\n\nUsage: jitso build <subcommand>"},
		{"build unknown subcommand", []string{"build", "show"}, 2, "", "jitso build: unknown subcommand \"show\"\n\nUsage: jitso build <subcommand>"},
		{"build --help", []string{"build", "--help"}, 0, "Usage: jitso build <subcommand>", ""},
		{"build -h", []string{"build", "-h"}, 0, "Usage: jitso build <subcommand>", ""},
		{"build list needs the project", []string{"build", "list"}, 3, "", "jitso: the cwd must not be read"},
		{"launch checks its arguments before the project", []string{"launch"}, 2, "", "JITSO REJECTED: --build is required"},
		{"launch --help prints the usage", []string{"launch", "--help"}, 0, "Usage: jitso launch --build <id> --brief <file> --mode direct|fleet [options]\n\nLaunches a worker", ""},
		{"launch -h with later words", []string{"launch", "-h", "--build", "x", "bogus"}, 0, "  --mcp-exclude <name>         leave one MCP server", ""},
		{"launch usage ends with --dry-run", []string{"launch", "--help"}, 0, "open nothing, write nothing\n", ""},
		{"launch help only as the first word", []string{"launch", "--build", "x", "--help"}, 2, "", `unknown argument "--help"`},
		{"launch value starting with two dashes", []string{"launch", "--build", "--dry-run"}, 2, "", "JITSO REJECTED: --build needs a value"},
		{"close keeps rejecting --help", []string{"close", "--help"}, 2, "", "JITSO REJECTED:"},
		{"hook needs a name", []string{"hook"}, 2, "", "jitso hook: expected guard"},
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

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("want a panic")
		}
	}()
	Register(Command{Name: "tree", Run: func(*Context, []string) error { return nil }})
}
