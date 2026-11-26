package catalog

import (
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// SummaryLine is one build as `build list` shows it:
// `- <id>: <model> <effort>. <description> requires: <fields or nothing>`.
// A part the file does not give is left out.
func SummaryLine(b *Build) string {
	return summaryOf(b.Raw, jsconv.String(b.Raw.Value("id")))
}

// summaryOf is the summary line of a decoded file under the given id text.
func summaryOf(m *Map, id string) string {
	requires := "nothing"
	if list, ok := m.Value("requires").([]any); ok && len(list) > 0 {
		requires = jsconv.Join(list, ", ")
	}
	line := "- " + id + ":"
	if with := strings.TrimSpace(textOf(m, "model") + " " + textOf(m, "effort")); with != "" {
		line += " " + with + "."
	}
	if description := textOf(m, "description"); description != "" {
		line += " " + description
	}
	return line + " requires: " + requires
}

// textOf is the String() of a key of a decoded file, "" when the key is
// missing or has no value.
func textOf(m *Map, key string) string {
	if v, ok := m.Get(key); ok && v != nil {
		return jsconv.String(v)
	}
	return ""
}

// DraftLine is one draft file as `build list --drafts` shows it: the summary
// line, then ` (draft <display path>)`, then ` shadowed by <layer> <display
// path>` when a saved build has the id of the draft, then ` invalid here:
// <problems joined by "; ">` when the file is not valid for this project. A
// file that is not a mapping, or has no id, still has a line: the id is the
// file name without its extension.
func DraftLine(f *File) string {
	id := jsconv.String(f.Raw.Value("id"))
	if f.Raw.Value("id") == jsconv.Undefined {
		id = strings.TrimSuffix(f.Name, buildsExtension)
	}
	line := summaryOf(f.Raw, id) + " (draft " + f.Display + ")"
	if f.ShadowedBy != nil {
		line += " shadowed by " + string(f.ShadowedBy.Layer) + " " + f.ShadowedBy.Display
	}
	if !f.Valid() {
		line += " invalid here: " + strings.Join(f.Problems, "; ")
	}
	return line
}

// List is one SummaryLine per launchable build, joined with newlines, in catalog
// order. It is "" for a catalog with no launchable build.
func (c *Catalog) List() string {
	launchable := c.Launchable()
	lines := make([]string, len(launchable))
	for i := range launchable {
		lines[i] = SummaryLine(&launchable[i])
	}
	return strings.Join(lines, "\n")
}
