package card

import "strings"

// Rules is the JITSO rules text a launch appends to the end of brief.md: a
// heading and three lines, joined with "\n", without a trailing newline. mode
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
	return strings.Join([]string{
		"## JITSO rules",
		launch,
		skills,
		"Leave `jitso/`, `.jitso/` and `~/.jitso/` alone except your own draft in `.jitso/drafts/`, and never run `jitso build save`.",
	}, "\n")
}
