package card

import (
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	const (
		head   = "## JITSO rules\n"
		launch = "Start other agent sessions only with the card's `launch:` line."
		budget = " The card's budget is your ceiling: near it, stop and report."
		direct = "Load the card's skills before any other work. You may ask the human."
		fleet  = "Load the card's skills before any other work. Never ask a human: send your questions and your final report (a count of done, failed, open) to the session that launched you."
		last   = "Leave `jitso/`, `.jitso/` and `~/.jitso/` alone except your own draft in `.jitso/drafts/`, and never run `jitso build save`." +
			"\nFind another session with `jitso fleet` (read only); your own row is the one with your launch id." +
			"\nBetween two Claude Code sessions use `SendMessage` to the session name; otherwise use `orca orchestration send --to <handle> --from <your handle> --subject <s> --body <b>`." +
			"\nPut your own handle and session name in every message, so the peer can answer with `orca orchestration reply --id <msg_id> --body <b>`."
	)
	tests := []struct {
		name      string
		mode      string
		hasBudget bool
		want      string
	}{
		{"direct with a budget", "direct", true, head + launch + budget + "\n" + direct + "\n" + last},
		{"direct without a budget", "direct", false, head + launch + "\n" + direct + "\n" + last},
		{"fleet with a budget", "fleet", true, head + launch + budget + "\n" + fleet + "\n" + last},
		{"fleet without a budget", "fleet", false, head + launch + "\n" + fleet + "\n" + last},
		{"any other mode is the fleet text", "", true, head + launch + budget + "\n" + fleet + "\n" + last},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Rules(tt.mode, tt.hasBudget)
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
			// Heading, three rules and three communication lines: under eight lines.
			if n := strings.Count(got, "\n"); n != 6 {
				t.Errorf("%d newlines, want 6", n)
			}
			for _, bad := range []string{"Load first", "ROUTE-SCOPE:", "\u2013", "\u2014"} {
				if strings.Contains(got, bad) {
					t.Errorf("contains %q", bad)
				}
			}
		})
	}
}
