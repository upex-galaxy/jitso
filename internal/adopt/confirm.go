package adopt

import (
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Prompt is the question asked on a terminal. It is printed without a newline.
const Prompt = "Apply these changes? [y/N] "

// Decision is what a run does after the preview.
type Decision int

// The decisions.
const (
	// DecideNothing: the plan is empty. No preview, no question.
	DecideNothing Decision = iota + 1
	// DecideDryRun: show the preview, write nothing.
	DecideDryRun
	// DecideApply: write.
	DecideApply
	// DecideAsk: ask the question on the terminal and read one line.
	DecideAsk
	// DecideRefuse: no --yes and nobody to ask. Exit 2, nothing written.
	DecideRefuse
	// DecideDeclined: the answer was not yes, or there was none. Exit 2,
	// nothing written.
	DecideDeclined
)

// Confirm is what the rule looks at: the shape of the plan and how the
// command was started.
type Confirm struct {
	// Empty: the plan has no effective change.
	Empty bool
	// DryRun is --dry-run; it wins over Yes.
	DryRun bool
	// Yes is --yes.
	Yes bool
	// Terminal: standard input is a terminal (internal/tty).
	Terminal bool
}

// Before is the decision that needs no answer: DecideNothing, DecideDryRun,
// DecideApply, DecideRefuse, or DecideAsk when the answer decides. In that
// order: an empty plan, a dry run, --yes, no terminal, otherwise ask.
func (c Confirm) Before() Decision {
	switch {
	case c.Empty:
		return DecideNothing
	case c.DryRun:
		return DecideDryRun
	case c.Yes:
		return DecideApply
	case !c.Terminal:
		return DecideRefuse
	}
	return DecideAsk
}

// Reply is the decision for one answer line. "y" and "yes" in any case, with
// surrounding blanks and the line break ignored, apply; anything else, an
// empty line and end of input without a line decline. A last line without a
// newline still counts (eof is true then, and the line is read as usual).
func Reply(line string, eof bool) Decision {
	if eof && strings.TrimSpace(line) == "" {
		return DecideDeclined
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return DecideApply
	}
	return DecideDeclined
}

// Decide is the whole rule as one pure function: Before, and when that asks,
// Reply on the given answer.
func Decide(c Confirm, line string, eof bool) Decision {
	d := c.Before()
	if d != DecideAsk {
		return d
	}
	return Reply(line, eof)
}

// Err is the error a decision ends the run with: a rejection (exit 2) for
// DecideRefuse and DecideDeclined, nil for every other decision.
func (d Decision) Err() error {
	switch d {
	case DecideRefuse:
		return fail.Rejected("this changes files: read the preview above, then run it again with --yes, or in a terminal to answer the prompt; nothing written")
	case DecideDeclined:
		return fail.Rejected("not applied; nothing written")
	}
	return nil
}
