package link

import (
	"reflect"
	"testing"

	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

func launch(id, handle, title string, ended bool) registry.Launch {
	l := registry.Launch{ID: id, Launch: registry.Event{"orca": map[string]any{"terminal": handle, "title": title}}}
	if ended {
		l.End = registry.Event{}
	}
	return l
}

func TestDirectory(t *testing.T) {
	d := NewDirectory(
		[]registry.Launch{
			launch("L1", "term_a", "[jitso:L1] one", false),
			launch("L2", "term_b", "[jitso:L2] two", true),
			// A running launch keeps a handle an ended one also names.
			launch("L3", "term_c", "[jitso:L3] three", false),
			launch("L4", "term_c", "[jitso:L4] four", true),
		},
		[]orca.TerminalInfo{
			{Handle: "term_a", Title: "shared"},
			// The tab of an ended launch is not its name.
			{Handle: "term_b", Title: "zsh"},
			// A terminal with a launch's mark is that launch.
			{Handle: "term_m", Title: "reopened [jitso:L1]"},
			{Handle: "term_x", Title: "shared"},
			{Handle: "term_y", Title: "zsh"},
		},
	)
	named := func(id, handle, name string, ended bool) registry.Launch {
		l := launch(id, handle, name+" [jitso:"+id+"]", ended)
		l.Launch["name"] = name
		return l
	}
	byName := NewDirectory(
		[]registry.Launch{
			named("L1", "term_a", "scout", false),
			named("L2", "term_b", "twin", false),
			named("L3", "term_c", "twin", false),
			named("L4", "term_d", "scout", true),
			named("L5", "term_e", "finished", true),
		},
		// A tab that shows the name of its own session adds nothing.
		[]orca.TerminalInfo{{Handle: "term_a", Title: "scout"}},
	)
	for ref, want := range map[string][]Session{
		"scout":               {{"L1", "term_a"}},
		"twin":                {{"L2", "term_b"}, {"L3", "term_c"}},
		"finished":            nil,
		"scout [jitso:L4]":    {{"L4", "term_d"}},
		"finished [jitso:L5]": {{"L5", "term_e"}},
	} {
		if got := byName.Match(ref); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v, want %v", ref, got, want)
		}
	}
	tests := []struct {
		ref  string
		want []Session
	}{
		{"L1", []Session{{"L1", "term_a"}}},
		{"L2", []Session{{"L2", "term_b"}}},
		{"term_c", []Session{{"L3", "term_c"}}},
		{"term_m", []Session{{"L1", "term_m"}}},
		{"reopened [jitso:L1]", []Session{{"L1", "term_m"}}},
		{"[jitso:L2] two", []Session{{"L2", "term_b"}}},
		{"zsh", []Session{{"", "term_y"}}},
		{"shared", []Session{{"L1", "term_a"}, {"", "term_x"}}},
		{"Shared", nil},
		{"nothing", nil},
	}
	for _, tt := range tests {
		if got := d.Match(tt.ref); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q: %v, want %v", tt.ref, got, tt.want)
		}
	}
}
