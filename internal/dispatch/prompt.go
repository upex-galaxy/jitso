package dispatch

import "regexp"

var placeholder = regexp.MustCompile(`\{\{(\w+)\}\}`)

// PromptParts are the values the dispatcher prompt template may name.
type PromptParts struct {
	// Catalog is one line per launchable build (catalog.Summary).
	Catalog string
	// Launcher is the path of this executable, the word the guarded launch
	// line starts with.
	Launcher string
	// BriefsDir is the folder the dispatcher writes its briefs to.
	BriefsDir string
}

// RenderPrompt fills {{CATALOG}}, {{LAUNCHER}} and {{BRIEFS_DIR}} in template.
// Any other {{NAME}} stays as written, and an inserted value is never scanned
// again, so a catalog description holding a placeholder stays literal.
func RenderPrompt(template string, parts PromptParts) string {
	values := map[string]string{"CATALOG": parts.Catalog, "LAUNCHER": parts.Launcher, "BRIEFS_DIR": parts.BriefsDir}
	return placeholder.ReplaceAllStringFunc(template, func(match string) string {
		if v, ok := values[match[2:len(match)-2]]; ok {
			return v
		}
		return match
	})
}
