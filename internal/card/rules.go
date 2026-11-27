package card

import "strings"

// Communication is the three lines about reaching another session that close
// every rules text: how to find a peer, how to send (and, after a SendMessage,
// which leaves no trace jitso can read, how to record it), how to answer. The
// text is fixed and the same in both modes.
var Communication = []string{
	"Find another session with `jitso fleet` (read only); your own row is the one with your launch id.",
	"Between two Claude Code sessions use `SendMessage` to the session name, then run `jitso link <your launch id> <peer>`; otherwise use `orca orchestration send --to <handle> --from <your handle> --subject <s> --body <b>`. If a `SendMessage` is held, declined or not delivered, send it again with `orca orchestration send`.",
	"Put your own handle and session name in every message, so the peer can answer with `orca orchestration reply --id <msg_id> --body <b>`.",
}

// Rules is the JITSO rules text a launch appends to the end of brief.md: a
// heading, three rules and the three Communication lines, joined with "\n",
// without a trailing newline. mode
// "direct" gives the direct text (the session may ask the human); any other
// value gives the fleet text (the session reports to the session that
// launched it). hasBudget adds the sentence that names the card's budget as
// the ceiling. It is pure.
func Rules(mode string, hasBudget bool) string {
	launch := "Start other agent sessions only with the card's `launch:` line."
	if hasBudget {
		launch += " The card's budget is your ceiling: near it, stop and report."
	}
	skills := "Load the card's skills before any other work. "
	if mode == "direct" {
		skills += "You may ask the human."
	} else {
		skills += "Never ask a human: send your questions and your final report (a count of done, failed, open) to the session that launched you."
	}
	lines := []string{
		"## JITSO rules",
		launch,
		skills,
		"Never hand-edit `.jitso/launches.jsonl` or `.jitso/launches/`.",
	}
	return strings.Join(append(lines, Communication...), "\n")
}
