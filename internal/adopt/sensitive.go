package adopt

import "strings"

// compactLines is the line count under which an existing sensitive file is
// taken as compact JSON: one line holds every key, so a diff of it prints the
// whole file.
const compactLines = 3

// The marker lines that stand in for a line of an existing sensitive file.
// Only the first character is a diff prefix; the text never holds file content.
const (
	maskComma   = " (a line of the existing file changes only by a comma; not shown)\n"
	maskRemoved = " (a line of the existing file is removed; not shown)\n"
	maskAdded   = " (a line is added that jitso did not write; not shown)\n"
)

// UnifiedSensitive is the diff of old to new for a file that may hold secrets,
// printed with no unchanged line. A diff line is only printed when jitso wrote
// it: a structural line (a lone brace or bracket), a blank one, or one that
// written reports as written by jitso (written takes the line trimmed of its
// spaces and of a trailing comma and may be nil). Any other line is a line of
// the user's file and shows as a marker: the comma marker when the same text
// is on the other side (the line only gains or loses a comma, one marker for
// the pair), else a removed or added marker. An existing file of fewer than
// three lines (compact JSON) holding a line of the user's prints one line
// saying so. Above MaxCells it prints only the count of changed lines.
func UnifiedSensitive(display string, old, new []byte, written func(line string) bool) string {
	return unifiedSensitive(display, old, new, written, MaxCells)
}

func unifiedSensitive(display string, old, new []byte, written func(line string) bool, maxCells int) string {
	return render(display, old, new, 0, FallbackSummary, maxCells, &guard{written: written})
}

type guard struct {
	written func(line string) bool
}

// trimLine is a line without its terminator, its surrounding spaces and a
// trailing comma.
func trimLine(line string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
}

// ours reports whether a trimmed line is one jitso prints.
func (g *guard) ours(t string) bool {
	switch t {
	case "", "{", "}", "[", "]", "{}", "[]":
		return true
	}
	return g.written != nil && g.written(t)
}

// compact reports whether the existing file is compact JSON: it has fewer than
// compactLines lines and one of them is not jitso's (a bare `{}` hides nothing,
// so its diff, which is all jitso's lines, still prints).
func (g *guard) compact(old []byte) bool {
	lines := splitLines(old)
	if len(lines) == 0 || len(lines) >= compactLines {
		return false
	}
	for _, l := range lines {
		if !g.ours(trimLine(l)) {
			return true
		}
	}
	return false
}

// mask rewrites the lines of ops that are not jitso's into markers.
func (g *guard) mask(ops []op, a, c []string) {
	oldSet, newSet := lineSet(a), lineSet(c)
	for i := 0; i < len(ops); {
		if ops[i].kind == opEq {
			i++
			continue
		}
		j := i
		for j < len(ops) && ops[j].kind != opEq {
			j++
		}
		g.maskRun(ops[i:j], oldSet, newSet)
		i = j
	}
}

// maskRun masks one run of changed lines. A removed and an added line of the
// same text are one user line that changed by a comma: the removed one shows
// the marker and the added one is left out.
func (g *guard) maskRun(run []op, oldSet, newSet map[string]bool) {
	dels, inss := map[string]int{}, map[string]int{}
	for _, o := range run {
		t := trimLine(o.line)
		if g.ours(t) {
			continue
		}
		switch {
		case o.kind == opDel && newSet[t]:
			dels[t]++
		case o.kind == opIns && oldSet[t]:
			inss[t]++
		}
	}
	paired := map[string]int{}
	for t, n := range dels {
		paired[t] = min(n, inss[t])
	}
	for k := range run {
		o := &run[k]
		t := trimLine(o.line)
		if g.ours(t) {
			continue
		}
		switch {
		case o.kind == opDel && newSet[t]:
			o.mark, o.line = '~', maskComma
		case o.kind == opIns && oldSet[t]:
			if paired[t] > 0 {
				paired[t]--
				o.hide = true
			} else {
				o.mark, o.line = '~', maskComma
			}
		case o.kind == opDel:
			o.line = maskRemoved
		default:
			o.line = maskAdded
		}
	}
}

func lineSet(lines []string) map[string]bool {
	set := make(map[string]bool, len(lines))
	for _, l := range lines {
		set[trimLine(l)] = true
	}
	return set
}
