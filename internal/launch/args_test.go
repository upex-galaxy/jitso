package launch

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	base := []string{"--build", "p", "--brief", "b.md", "--mode", "direct"}
	with := func(extra ...string) []string { return append(append([]string(nil), base...), extra...) }
	tests := []struct {
		name   string
		argv   []string
		reason string
	}{
		{"ok", base, ""},
		{"no flags", nil, "--build is required"},
		{"unknown word", with("positional"), `unknown argument "positional"`},
		{"short flag", with("-h"), `unknown argument "-h"`},
		{"help", []string{"--help"}, `unknown argument "--help"`},
		{"missing value", with("--worktree"), "--worktree needs a value"},
		{"value may not start with two dashes", []string{"--build", "--dry-run"}, "--build needs a value"},
		{"two dashes after a valid flag", with("--run", "--task"), "--run needs a value"},
		{"two dashes on the field flag", with("--field", "--dry-run"), "--field needs a value"},
		{"two dashes on the exclude flag", with("--mcp-exclude", "--field"), "--mcp-exclude needs a value"},
		{"two dashes alone", with("--brief-language", "--"), "--brief-language needs a value"},
		{"one dash is still a value", with("--run", "-x"), ""},
		{"a value in the middle of the words", []string{"--build", "p", "--brief", "--mode", "direct"}, "--brief needs a value"},
		{"field without equals", with("--field", "ticket_key"), "--field must look like name=value"},
		{"field with a leading equals", with("--field", "=x"), "--field must look like name=value"},
		{"missing brief", []string{"--build", "p", "--mode", "direct"}, "--brief is required"},
		{"bad brief language", with("--brief-language", "EN"), "--brief-language must be en, es or any"},
		{"empty brief language", with("--brief-language", ""), "--brief-language must be en, es or any"},
		{"missing mode", []string{"--build", "p", "--brief", "b"}, "--mode must be direct or fleet"},
		{"legacy alias", []string{"--build", "p", "--brief", "b", "--mode", "directo"}, "--mode must be direct or fleet"},
		{"language before mode", []string{"--build", "p", "--brief", "b", "--mode", "x", "--brief-language", "fr"}, "--brief-language must be en, es or any"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.argv)
			if tt.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != "JITSO REJECTED: "+tt.reason {
				t.Fatalf("got %v, want %q", err, tt.reason)
			}
		})
	}
}

func TestParseValues(t *testing.T) {
	a, err := Parse([]string{"--dry-run", "--build", "a", "--build", "b", "--brief", "x.md", "--mode", "fleet", "--run", "",
		"--mcp-exclude", "dbhub", "--mcp-exclude", "DBHUB", "--field", "k=v=w", "--field", "k=z", "--brief-language", "any"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Build != "b" || a.Brief != "x.md" || a.Mode != ModeFleet || !a.DryRun || a.BriefLanguage != "any" {
		t.Errorf("args %+v", a)
	}
	if a.Run == nil || *a.Run != "" || a.Task != nil || a.Parent != nil || a.Worktree != nil {
		t.Error("an empty --run is given; the others are not")
	}
	if !reflect.DeepEqual(a.MCPExclude, []string{"dbhub", "DBHUB"}) || a.Fields.Get("k") != "z" || a.Fields.Len() != 1 {
		t.Errorf("lists %q %v", a.MCPExclude, a.Fields.Keys())
	}
	if d, _ := Parse([]string{"--build", "p", "--brief", "b", "--mode", "direct"}); d.BriefLanguage != "en" {
		t.Errorf("default language %q", d.BriefLanguage)
	}
}

func TestCheck(t *testing.T) {
	s := func(v string) *string { return &v }
	ok := Args{Build: "haiku-explorer", Mode: ModeDirect}
	tests := []struct {
		name   string
		edit   func(*Args)
		reason string
	}{
		{"ok", func(*Args) {}, ""},
		{"empty build", func(a *Args) { a.Build = "" }, "invalid value for --build"},
		{"empty run is checked", func(a *Args) { a.Run = s("") }, "invalid value for --run"},
		{"hostile task", func(a *Args) { a.Task = s("x;ls") }, "invalid value for --task"},
		{"parent shape", func(a *Args) { a.Parent = s("L2026") }, "invalid value for --parent"},
		{"worktree dots", func(a *Args) { a.Worktree = s("a..b") }, "invalid value for --worktree"},
		{"mcp exclude", func(a *Args) { a.MCPExclude = []string{"ok", "db.hub"} }, "invalid value for --mcp-exclude"},
		{"build before run", func(a *Args) { a.Build, a.Run = "X", s("") }, "invalid value for --build"},
		{"fleet needs a run", func(a *Args) { a.Mode = ModeFleet }, "fleet mode requires --run"},
		{"fleet rejects a task", func(a *Args) { a.Mode, a.Run, a.Task = ModeFleet, s("r"), s("t") }, "--task is not accepted in fleet mode: JITSO creates the Task from the brief"},
		{"fleet with a run passes", func(a *Args) { a.Mode, a.Run = ModeFleet, s("r") }, ""},
		{"direct ignores run and task", func(a *Args) { a.Run = s("r") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := ok
			tt.edit(&a)
			err := a.Check()
			if tt.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.HasSuffix(err.Error(), tt.reason) {
				t.Fatalf("got %v, want %q", err, tt.reason)
			}
		})
	}
}
