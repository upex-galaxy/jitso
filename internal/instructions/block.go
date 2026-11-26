// Package instructions keeps one delimited, reversible JITSO block in the
// instruction files agents read (`jitso setup instructions`, defect N-18).
//
// It is pure: every function takes the bytes of a file and returns bytes.
// Reading and writing files, the preview, the backup and the confirmation
// belong to internal/adopt and to the command.
//
// The block sits between two marker lines:
//
//	<!-- BEGIN JITSO block=<n> added=<new|0|1|2>: ... -->
//	<!-- END JITSO block=<n> -->
//
// A marker is recognized only at the start of a line and outside a fenced
// code block. `added` records how many line breaks jitso put before the
// block (new: jitso created the file), so Remove gives the original bytes
// back exactly.
package instructions

import (
	"strconv"
	"strings"
)

// Number is the block number this version writes. A block with another number
// is replaced in place.
const Number = 2

// Command is the command named in the BEGIN line.
const Command = "jitso setup instructions"

// body is the text between the markers, one line per element, in the order
// they are written. It holds no @ outside backticks (Claude Code would read
// one as an import), no count and no date.
var body = []string{
	"## JITSO",
	"",
	"In a project that has `jitso/jitso.yaml`:",
	"",
	"- Start worker sessions only with `jitso launch`. The guard denies the other ways of starting an agent.",
	"- A launch takes a build. When no saved one fits (`jitso build list`), compose one just in time with `jitso build draft -`; `jitso arsenal` lists what it may name. Saving it with `jitso build save` is optional.",
	"- Load the `jitso` skill before you launch.",
}

// The values `added` can take.
const (
	AddedNew = "new"
	Added0   = "0"
	Added1   = "1"
	Added2   = "2"
)

// beginLine is the BEGIN marker for the current block number.
func beginLine(added string) string {
	return "<!-- BEGIN JITSO block=" + strconv.Itoa(Number) + " added=" + added + `: managed by "` + Command + `"; edits inside this block are replaced -->`
}

// endLine is the END marker for the current block number.
func endLine() string {
	return "<!-- END JITSO block=" + strconv.Itoa(Number) + " -->"
}

// Render is the block as written into a file: from the BEGIN line through the
// END line, lines joined by unit ("\n" or "\r\n"), with no break after the END
// line.
func Render(added, unit string) string {
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, beginLine(added))
	lines = append(lines, body...)
	lines = append(lines, endLine())
	return strings.Join(lines, unit)
}
