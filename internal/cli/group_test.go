package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestGroup(t *testing.T) {
	var got []string
	g := Group{
		Name:  "install",
		Usage: "Usage: jitso install <what>\n\n  skill  the skill",
		Subs: []GroupSub{
			{Name: "skill", Run: func(ctx *Context, args []string) error {
				got = append([]string{"skill"}, args...)
				return nil
			}},
			{Name: "boom", Run: func(*Context, []string) error { return errors.New("boom") }},
		},
	}
	tests := []struct {
		name   string
		group  Group
		args   []string
		code   int
		stdout string
		stderr string
		ran    string
	}{
		{"missing", g, nil, 2, "", "jitso install: missing subcommand\n\nUsage: jitso install <what>\n\n  skill  the skill\n", ""},
		{"missing with its own text", withMissing(g, "missing what to install (skill)"), nil, 2, "", "jitso install: missing what to install (skill)\n\nUsage: jitso install <what>\n\n  skill  the skill\n", ""},
		{"--help", g, []string{"--help"}, 0, "Usage: jitso install <what>\n\n  skill  the skill\n", "", ""},
		{"-h with more words", g, []string{"-h", "skill"}, 0, "Usage: jitso install <what>\n\n  skill  the skill\n", "", ""},
		{"unknown", g, []string{"hook"}, 2, "", "jitso install: unknown subcommand \"hook\"\n\nUsage: jitso install <what>\n\n  skill  the skill\n", ""},
		{"case matters", g, []string{"Skill"}, 2, "", "jitso install: unknown subcommand \"Skill\"\n\n", ""},
		{"a flag is not a subcommand", g, []string{"--yes"}, 2, "", "jitso install: unknown subcommand \"--yes\"\n\n", ""},
		{"the subcommand gets the rest", g, []string{"skill", "--scope", "user"}, 0, "", "", "skill --scope user"},
		{"the subcommand's error comes through", g, []string{"boom"}, 3, "", "jitso: boom\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got = nil
			var out, errOut bytes.Buffer
			code := report(&errOut, tt.group.Run(&Context{Sys: System{Stdout: &out, Stderr: &errOut}}, tt.args))
			if code != tt.code || out.String() != tt.stdout || !strings.HasPrefix(errOut.String(), tt.stderr) {
				t.Errorf("code %d stdout %q stderr %q", code, out.String(), errOut.String())
			}
			if strings.Join(got, " ") != tt.ran {
				t.Errorf("ran %q, want %q", strings.Join(got, " "), tt.ran)
			}
		})
	}
}

func withMissing(g Group, text string) Group {
	g.Missing = text
	return g
}

func TestGroupCommand(t *testing.T) {
	ran := false
	g := Group{Name: "sample", Usage: "u", Subs: []GroupSub{{Name: "hook", Run: func(*Context, []string) error { ran = true; return nil }}}}
	c := g.Command(82, "set things up")
	if c.Name != "sample" || c.Order != 82 || c.Summary != "set things up" || c.Run == nil {
		t.Fatalf("command = %+v", c)
	}
	if err := c.Run(&Context{}, []string{"hook"}); err != nil || !ran {
		t.Errorf("err %v ran %v", err, ran)
	}
	if err := c.Run(&Context{}, nil); fail.Code(err) != 2 {
		t.Errorf("err = %v", err)
	}
	if _, ok := Lookup("sample"); ok {
		t.Error("Command must not register")
	}
}
