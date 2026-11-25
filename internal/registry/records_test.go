package registry

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/usage"
)

func TestLinkEvent(t *testing.T) {
	handle, id := "term_x", "L20261125T100000-a1a1"
	line, err := Encode(&LinkEvent{TS: ts, From: LinkEnd{ID: &id}, To: LinkEnd{Handle: &handle}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":2,"event":"link","ts":"` + ts + `","from":{"id":"` + id + `","handle":null},"to":{"id":null,"handle":"term_x"}}`
	if string(line) != want {
		t.Errorf("%s, want %s", line, want)
	}
	events := Parse(append(line, '\n'))
	if got := Links(events); len(got) != 1 || got[0] != (Link{FromID: id, ToHandle: handle, TS: ts}) {
		t.Errorf("links %+v", got)
	}
	if launches := Fold(events); len(launches) != 0 {
		t.Errorf("a link is no launch: %+v", launches)
	}
	empty, unsafe := "", "not an id"
	for name, e := range map[string]*LinkEvent{
		"no side":      {TS: ts, From: LinkEnd{ID: &id}},
		"empty handle": {TS: ts, From: LinkEnd{ID: &id}, To: LinkEnd{Handle: &empty}},
		"unsafe id":    {TS: ts, From: LinkEnd{ID: &unsafe}, To: LinkEnd{Handle: &handle}},
		"no time":      {From: LinkEnd{ID: &id}, To: LinkEnd{Handle: &handle}},
	} {
		if _, err := Encode(e); err == nil {
			t.Errorf("%s was written", name)
		}
	}
	for _, sides := range []string{`"from":"x","to":{"handle":"a"}`, `"to":{"handle":"a"}`, `"from":{"handle":7},"to":{"handle":"a"}`} {
		if got := Parse([]byte(`{"v":2,"event":"link","ts":"` + ts + `",` + sides + `}`)); len(got) != 0 {
			t.Errorf("%s was read", sides)
		}
	}
}

func TestEndEventTokensKey(t *testing.T) {
	tests := []struct {
		name   string
		tokens *Tokens
		want   string
	}{
		{"left out", nil, ""},
		{"null", &Tokens{}, `"tokens":null`},
		{"totals", &Tokens{Totals: &usage.Totals{Input: 1, Messages: 2}}, `"tokens":{"input":1,"cache_write":0,"cache_read":0,"output":0,"messages":2,"subagent_files":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(&EndEvent{ID: "x", Tokens: tt.tokens})
			if err != nil {
				t.Fatal(err)
			}
			has := strings.Contains(string(raw), `"tokens"`)
			if has != (tt.want != "") || (tt.want != "" && !strings.Contains(string(raw), tt.want)) {
				t.Errorf("%s, want %q", raw, tt.want)
			}
		})
	}
}
