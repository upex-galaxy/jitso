package catalog

import (
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// SummaryLine is one build as `presets list` and the dispatcher prompt show
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

// Summary is one SummaryLine per launchable build, joined with newlines, in
// file name order. It is "" for a catalog with no launchable build.
func (c *Catalog) Summary() string {
	launchable := c.Launchable()
	lines := make([]string, len(launchable))
	for i := range launchable {
		lines[i] = SummaryLine(&launchable[i])
	}
	return strings.Join(lines, "\n")
}
