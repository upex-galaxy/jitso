package cli

import (
	"reflect"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestParseFlags(t *testing.T) {
	all := FlagScope | FlagRemove | FlagForce | FlagAbsolute | FlagDryRun | FlagYes
	tests := []struct {
		name   string
		args   []string
		accept FlagSet
		want   Flags
		rest   []string
		err    string
	}{
		{"none", nil, all, Flags{}, nil, ""},
		{"every flag", []string{"--scope", "user", "--remove", "--force", "--absolute", "--dry-run", "--yes"}, all,
			Flags{Scope: "user", Remove: true, Force: true, Absolute: true, DryRun: true, Yes: true}, nil, ""},
		{"scope project", []string{"--scope", "project"}, all, Flags{Scope: "project"}, nil, ""},
		{"last scope wins", []string{"--scope", "user", "--scope", "project"}, all, Flags{Scope: "project"}, nil, ""},
		{"a flag twice", []string{"--yes", "--yes"}, all, Flags{Yes: true}, nil, ""},
		{"positional words are returned in order", []string{"a", "--yes", "b"}, all, Flags{Yes: true}, []string{"a", "b"}, ""},
		{"scope value is not positional", []string{"--scope", "user", "x"}, all, Flags{Scope: "user"}, []string{"x"}, ""},
		{"only accepted flags", []string{"--absolute", "--remove", "--dry-run", "--yes"}, FlagAbsolute | FlagRemove | FlagDryRun | FlagYes,
			Flags{Absolute: true, Remove: true, DryRun: true, Yes: true}, nil, ""},

		{"unknown flag", []string{"--nope"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "--nope"`},
		{"flag not declared", []string{"--force"}, FlagYes, Flags{}, nil, `JITSO REJECTED: unknown argument "--force"`},
		{"scope not declared", []string{"--scope", "user"}, FlagYes, Flags{}, nil, `JITSO REJECTED: unknown argument "--scope"`},
		{"equals form", []string{"--scope=user"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "--scope=user"`},
		{"short flag", []string{"-y"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "-y"`},
		{"-h is not a flag here", []string{"-h"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "-h"`},
		{"bare dashes", []string{"--"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "--"`},
		{"scope without a value", []string{"--scope"}, all, Flags{}, nil, "JITSO REJECTED: --scope needs a value"},
		{"scope value starts with --", []string{"--scope", "--remove"}, all, Flags{}, nil, "JITSO REJECTED: --scope needs a value"},
		{"scope other", []string{"--scope", "global"}, all, Flags{}, nil, "JITSO REJECTED: --scope must be user or project"},
		{"scope empty", []string{"--scope", ""}, all, Flags{}, nil, "JITSO REJECTED: --scope must be user or project"},
		{"scope case matters", []string{"--scope", "User"}, all, Flags{}, nil, "JITSO REJECTED: --scope must be user or project"},
		{"first problem wins", []string{"--bad", "--scope"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "--bad"`},

		{"harness list in plan order", []string{"--harness", "pi,claude,pi"}, all | FlagHarness, Flags{Harnesses: []string{"claude", "pi"}}, nil, ""},
		{"harness all", []string{"--harness", "codex,all"}, all | FlagHarness, Flags{Harnesses: []string{"claude", "codex", "opencode", "pi"}}, nil, ""},
		{"last harness wins", []string{"--harness", "pi", "--harness", "codex"}, all | FlagHarness, Flags{Harnesses: []string{"codex"}}, nil, ""},
		{"harness not declared", []string{"--harness", "pi"}, all, Flags{}, nil, `JITSO REJECTED: unknown argument "--harness"`},
		{"harness without a value", []string{"--harness"}, all | FlagHarness, Flags{}, nil, "JITSO REJECTED: --harness needs a value"},
		{"harness value starts with --", []string{"--harness", "--yes"}, all | FlagHarness, Flags{}, nil, "JITSO REJECTED: --harness needs a value"},
		{"harness empty", []string{"--harness", ""}, all | FlagHarness, Flags{}, nil, "JITSO REJECTED: --harness must be a comma list of claude, codex, opencode and pi, or all"},
		{"harness empty name", []string{"--harness", "claude,,pi"}, all | FlagHarness, Flags{}, nil, "JITSO REJECTED: --harness must be a comma list of claude, codex, opencode and pi, or all"},
		{"harness unknown", []string{"--harness", "Claude"}, all | FlagHarness, Flags{}, nil, "JITSO REJECTED: --harness must be a comma list of claude, codex, opencode and pi, or all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rest, err := ParseFlags(tt.args, tt.accept)
			if tt.err != "" {
				if fail.Code(err) != 2 || fail.Text(err) != tt.err {
					t.Fatalf("code %d text %q, want exit 2 %q", fail.Code(err), fail.Text(err), tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) || !reflect.DeepEqual(rest, tt.rest) {
				t.Errorf("got %+v %q, want %+v %q", got, rest, tt.want, tt.rest)
			}
		})
	}
}

func TestRejectExtra(t *testing.T) {
	if err := RejectExtra(nil); err != nil {
		t.Errorf("nil: %v", err)
	}
	err := RejectExtra([]string{"x", "y"})
	if fail.Code(err) != 2 || fail.Text(err) != `JITSO REJECTED: unknown argument "x"` {
		t.Errorf("got %q", fail.Text(err))
	}
}
