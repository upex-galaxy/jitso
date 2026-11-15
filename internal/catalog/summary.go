package catalog

import (
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// SummaryLine is one build as `build list` and the dispatcher prompt show
// it: `- <id>: <model> <effort>. <description> requires: <fields or nothing>`.
// Values are rendered with String(), so a missing description reads
// "undefined", as in the oracle.
func SummaryLine(b *Build) string {
	m := b.Raw
	requires := "nothing"
	if list, ok := m.Value("requires").([]any); ok && len(list) > 0 {
		requires = jsconv.Join(list, ", ")
	}
	return "- " + jsconv.String(m.Value("id")) + ": " + jsconv.String(m.Value("model")) + " " +
		jsconv.String(m.Value("effort")) + ". " + jsconv.String(m.Value("description")) + " requires: " + requires
}

// ListLine is one build as `build list` shows it: the SummaryLine, plus
// ` (from <layer> <display path>)` for a build of a parent or user folder. A
// build of the project itself has no suffix.
func ListLine(b *Build) string {
	line := SummaryLine(b)
	if b.LayerWord() != LayerRepo {
		line += " (from " + string(b.Layer) + " " + b.Display + ")"
	}
	return line
}

// List is one ListLine per launchable build, joined with newlines, in catalog
// order. It is "" for a catalog with no launchable build.
func (c *Catalog) List() string {
	launchable := c.Launchable()
	lines := make([]string, len(launchable))
	for i := range launchable {
		lines[i] = ListLine(&launchable[i])
	}
	return strings.Join(lines, "\n")
}

// Summary is one SummaryLine per launchable build, joined with newlines, in
// catalog order, without the origin of a parent or user build: the dispatcher
// model does not need it. It is "" for a catalog with no launchable build.
func (c *Catalog) Summary() string {
	launchable := c.Launchable()
	lines := make([]string, len(launchable))
	for i := range launchable {
		lines[i] = SummaryLine(&launchable[i])
	}
	return strings.Join(lines, "\n")
}
