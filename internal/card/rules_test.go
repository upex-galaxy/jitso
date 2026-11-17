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
		last   = "Leave `jitso/` and `.jitso/` alone except your own draft in `.jitso/drafts/`, and never run `jitso build save`."
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
			if n := strings.Count(got, "\n"); n != 3 {
				t.Errorf("%d newlines, want 3", n)
			}
			for _, bad := range []string{"Load first", "ROUTE-SCOPE:", "\u2013", "\u2014"} {
				if strings.Contains(got, bad) {
					t.Errorf("contains %q", bad)
				}
			}
		})
	}
}
