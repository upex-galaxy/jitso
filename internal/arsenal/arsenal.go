// Package arsenal is what `jitso arsenal` prints: every item a build may name
// in this project, one line per item, and with --all the items this machine
// holds that no allowlist names yet. Render is pure; Collect reads the files.
//
// It prints names, transports and key names only. No value of an env or a
// header is ever part of an Item, so none can be printed.
package arsenal

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Item is one line: its kind, the name as a build writes it, and a detail.
type Item struct {
	Kind   string
	Name   string
	Detail string
}

// Inventory is everything one arsenal prints.
type Inventory struct {
	// Header is a line printed before the items, "" for none.
	Header string
	// Allowed are the items a build may name, in print order, with a line for
	// each open allowlist.
	Allowed []Item
	// Candidates are the items no allowlist names (--all only).
	Candidates []Item
}

// Lines are the output lines; candidates only with all.
func (inv Inventory) Lines(all bool) []string {
	items := inv.Allowed
	if all {
		items = append(append([]Item(nil), items...), inv.Candidates...)
	}
	width := 0
	for _, it := range items {
		if n := utf8.RuneCountInString(it.Name); n > width {
			width = n
		}
	}
	lines := make([]string, 0, len(items)+1)
	if inv.Header != "" {
		lines = append(lines, inv.Header)
	}
	for _, it := range items {
		line := fmt.Sprintf("%-8s %s", it.Kind, it.Name)
		if it.Detail != "" {
			line += strings.Repeat(" ", width-utf8.RuneCountInString(it.Name)+2) + it.Detail
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

// MaxSummary is the longest skill summary printed, in characters.
const MaxSummary = 120

// Summary is the first sentence of a skill description, at most MaxSummary
// characters: a longer one is cut and ends with "...".
func Summary(description string) string {
	s := strings.Join(strings.Fields(description), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	if utf8.RuneCountInString(s) <= MaxSummary {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:MaxSummary-3]), " ") + "..."
}

// Description is the description of a SKILL.md: the value of the
// description key of its front matter, a plain or quoted scalar on one line,
// or a folded or literal block below it. "" when there is none.
func Description(skillMD string) string {
	lines := strings.Split(strings.ReplaceAll(skillMD, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		value, ok := strings.CutPrefix(line, "description:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch {
		case value == "" || strings.HasPrefix(value, ">") || strings.HasPrefix(value, "|"):
			var parts []string
			for _, next := range lines[i+1:] {
				if next != "" && next[0] != ' ' && next[0] != '\t' {
					break
				}
				parts = append(parts, strings.TrimSpace(next))
			}
			return strings.Join(parts, " ")
		case len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"':
			return strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
		case len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'':
			return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
		return value
	}
	return ""
}

// Transport is the transport of an MCP server definition: its type, else
// stdio for one with a command, http for one with a url, unknown otherwise.
func Transport(def map[string]any) string {
	if t, ok := def["type"].(string); ok && t != "" {
		return t
	}
	if _, ok := def["command"].(string); ok {
		return "stdio"
	}
	if _, ok := def["url"].(string); ok {
		return "http"
	}
	return "unknown"
}
