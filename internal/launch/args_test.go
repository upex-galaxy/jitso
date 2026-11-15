package launch

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	base := []string{"--preset", "p", "--brief", "b.md", "--mode", "direct"}
	with := func(extra ...string) []string { return append(append([]string(nil), base...), extra...) }
	tests := []struct {
		name   string
		argv   []string
		reason string
	}{
		{"ok", base, ""},
		{"no flags", nil, "--preset is required"},
		{"unknown word", with("positional"), `unknown argument "positional"`},
		{"short flag", with("-h"), `unknown argument "-h"`},
		{"help", []string{"--help"}, `unknown argument "--help"`},
		{"missing value", with("--worktree"), "--worktree needs a value"},
		{"value is the next word", []string{"--preset", "--dry-run"}, "--brief is required"},
		{"field without equals", with("--field", "ticket_key"), "--field must look like name=value"},
		{"field with a leading equals", with("--field", "=x"), "--field must look like name=value"},
		{"missing brief", []string{"--preset", "p", "--mode", "direct"}, "--brief is required"},
		{"bad brief language", with("--brief-language", "EN"), "--brief-language must be en, es or any"},
		{"empty brief language", with("--brief-language", ""), "--brief-language must be en, es or any"},
		{"missing mode", []string{"--preset", "p", "--brief", "b"}, "--mode must be direct or fleet"},
		{"legacy alias", []string{"--preset", "p", "--brief", "b", "--mode", "directo"}, "--mode must be direct or fleet"},
		{"language before mode", []string{"--preset", "p", "--brief", "b", "--mode", "x", "--brief-language", "fr"}, "--brief-language must be en, es or any"},
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
	a, err := Parse([]string{"--dry-run", "--preset", "a", "--preset", "b", "--brief", "x.md", "--mode", "fleet", "--run", "",
		"--mcp-exclude", "dbhub", "--mcp-exclude", "DBHUB", "--field", "k=v=w", "--field", "k=z", "--brief-language", "any"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Preset != "b" || a.Brief != "x.md" || a.Mode != ModeFleet || !a.DryRun || a.BriefLanguage != "any" {
		t.Errorf("args %+v", a)
	}
	if a.Run == nil || *a.Run != "" || a.Task != nil || a.Parent != nil || a.Worktree != nil {
		t.Error("an empty --run is given; the others are not")
	}
	if !reflect.DeepEqual(a.MCPExclude, []string{"dbhub", "DBHUB"}) || a.Fields.Get("k") != "z" || a.Fields.Len() != 1 {
		t.Errorf("lists %q %v", a.MCPExclude, a.Fields.Keys())
	}
	if d, _ := Parse([]string{"--preset", "p", "--brief", "b", "--mode", "direct"}); d.BriefLanguage != "en" {
		t.Errorf("default language %q", d.BriefLanguage)
	}
}

func TestCheck(t *testing.T) {
	s := func(v string) *string { return &v }
	ok := Args{Preset: "haiku-explorer", Mode: ModeDirect}
	tests := []struct {
		name   string
		edit   func(*Args)
		reason string
	}{
		{"ok", func(*Args) {}, ""},
		{"empty preset", func(a *Args) { a.Preset = "" }, "invalid value for --preset"},
		{"empty run is checked", func(a *Args) { a.Run = s("") }, "invalid value for --run"},
		{"hostile task", func(a *Args) { a.Task = s("x;ls") }, "invalid value for --task"},
		{"parent shape", func(a *Args) { a.Parent = s("L2026") }, "invalid value for --parent"},
		{"worktree dots", func(a *Args) { a.Worktree = s("a..b") }, "invalid value for --worktree"},
		{"mcp exclude", func(a *Args) { a.MCPExclude = []string{"ok", "db.hub"} }, "invalid value for --mcp-exclude"},
		{"preset before run", func(a *Args) { a.Preset, a.Run = "X", s("") }, "invalid value for --preset"},
		{"fleet needs run and task", func(a *Args) { a.Mode, a.Run = ModeFleet, s("r") }, "fleet mode requires --run and --task"},
		{"fields come after the fleet check", func(a *Args) { a.Mode = ModeFleet; a.Run, a.Task = s("r"), s("t") }, ""},
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
