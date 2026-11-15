package adopt

import (
	"fmt"
	"strings"
)

// MaxCells caps the dynamic programming table of the line diff. The cells are
// counted on the part of the two files left after their common first and last
// lines are set aside: (old lines + 1) * (new lines + 1).
const MaxCells = 4_000_000

// TooLarge is what Unified prints when the diff needs more than MaxCells.
type TooLarge int

const (
	// FallbackHunk prints one hunk: everything between the common first and
	// last lines as removed, then as added.
	FallbackHunk TooLarge = iota
	// FallbackSummary prints one line, "(<path>: <n> lines change, too large
	// to show)", and no file content. For files that may hold secrets.
	FallbackSummary
)

// Unified is the unified diff of old to new for the file shown as display,
// with context unchanged lines around each hunk (0 is allowed, then no
// unchanged line is printed). A nil old is an absent file (--- /dev/null) and
// a nil new is a removed one (+++ /dev/null); an empty non-nil slice is an
// existing empty file. It returns "" when nothing changes. Above MaxCells it
// falls back to one prefix/suffix hunk.
func Unified(display string, old, new []byte, context int) string {
	return UnifiedWith(display, old, new, context, FallbackHunk)
}

// UnifiedWith is Unified with the policy for a diff above MaxCells.
func UnifiedWith(display string, old, new []byte, context int, tooLarge TooLarge) string {
	return unified(display, old, new, context, tooLarge, MaxCells)
}

func unified(display string, old, new []byte, context int, tooLarge TooLarge, maxCells int) string {
	if (old == nil) == (new == nil) && string(old) == string(new) {
		return ""
	}
	if context < 0 {
		context = 0
	}
	var b strings.Builder
	if old == nil {
		b.WriteString("--- /dev/null\n")
	} else {
		b.WriteString("--- a/" + display + "\n")
	}
	if new == nil {
		b.WriteString("+++ /dev/null\n")
	} else {
		b.WriteString("+++ b/" + display + "\n")
	}
	a, c := splitLines(old), splitLines(new)
	ops, fell := diffOps(a, c, maxCells)
	if fell && tooLarge == FallbackSummary {
		return fmt.Sprintf("(%s: %d lines change, too large to show)\n", display, changedLines(ops))
	}
	writeHunks(&b, ops, context)
	return b.String()
}

// splitLines cuts text into lines that keep their own terminator, so a last
// line without a newline differs from the same line with one, and CRLF stays.
func splitLines(data []byte) []string {
	var lines []string
	s := string(data)
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}

// LineCount is the number of lines of data: its newlines, plus one when the
// last line has none. An empty file has 0.
func LineCount(data []byte) int { return len(splitLines(data)) }

type opKind byte

const (
	opEq  opKind = ' '
	opDel opKind = '-'
	opIns opKind = '+'
)

type op struct {
	kind opKind
	line string
}

// diffOps is the edit script from a to c. It sets the common first and last
// lines aside, runs the longest common subsequence on the rest, and when that
// table would pass maxCells it returns the prefix/suffix script instead
// (fell reports that).
func diffOps(a, c []string, maxCells int) (ops []op, fell bool) {
	pre := 0
	for pre < len(a) && pre < len(c) && a[pre] == c[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(c)-pre && a[len(a)-1-suf] == c[len(c)-1-suf] {
		suf++
	}
	ma, mc := a[pre:len(a)-suf], c[pre:len(c)-suf]
	ops = make([]op, 0, len(a)+len(c))
	for _, l := range a[:pre] {
		ops = append(ops, op{opEq, l})
	}
	switch {
	case len(ma) == 0 || len(mc) == 0 || (len(ma)+1)*(len(mc)+1) <= maxCells:
		ops = append(ops, lcsOps(ma, mc)...)
	default:
		fell = true
		for _, l := range ma {
			ops = append(ops, op{opDel, l})
		}
		for _, l := range mc {
			ops = append(ops, op{opIns, l})
		}
	}
	for _, l := range a[len(a)-suf:] {
		ops = append(ops, op{opEq, l})
	}
	return ops, fell
}

// lcsOps is the edit script from a to c by dynamic programming on the lengths
// of the longest common suffixes. Where two scripts are equally long it
// removes before it adds, and it keeps the earliest matching lines, so the
// output is deterministic.
func lcsOps(a, c []string) []op {
	n, m := len(a), len(c)
	if n == 0 || m == 0 {
		ops := make([]op, 0, n+m)
		for _, l := range a {
			ops = append(ops, op{opDel, l})
		}
		for _, l := range c {
			ops = append(ops, op{opIns, l})
		}
		return ops
	}
	w := m + 1
	table := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == c[j]:
				table[i*w+j] = table[(i+1)*w+j+1] + 1
			case table[(i+1)*w+j] >= table[i*w+j+1]:
				table[i*w+j] = table[(i+1)*w+j]
			default:
				table[i*w+j] = table[i*w+j+1]
			}
		}
	}
	ops := make([]op, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == c[j]:
			ops = append(ops, op{opEq, a[i]})
			i++
			j++
		case table[(i+1)*w+j] >= table[i*w+j+1]:
			ops = append(ops, op{opDel, a[i]})
			i++
		default:
			ops = append(ops, op{opIns, c[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{opDel, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{opIns, c[j]})
	}
	return ops
}

func changedLines(ops []op) int {
	n := 0
	for _, o := range ops {
		if o.kind != opEq {
			n++
		}
	}
	return n
}

// writeHunks prints the hunks of an edit script: one per run of changes, runs
// closer than two contexts apart share a hunk.
func writeHunks(b *strings.Builder, ops []op, context int) {
	// Runs of changes as [start, end) indexes into ops.
	type run struct{ start, end int }
	var runs []run
	for i := 0; i < len(ops); {
		if ops[i].kind == opEq {
			i++
			continue
		}
		j := i
		for j < len(ops) && ops[j].kind != opEq {
			j++
		}
		runs = append(runs, run{i, j})
		i = j
	}
	if len(runs) == 0 {
		return
	}
	// Merge runs separated by no more than two contexts of unchanged lines.
	groups := []run{runs[0]}
	for _, r := range runs[1:] {
		last := &groups[len(groups)-1]
		if r.start-last.end <= 2*context {
			last.end = r.end
		} else {
			groups = append(groups, r)
		}
	}
	// aPos[i], cPos[i]: how many old and new lines come before ops[i].
	aPos := make([]int, len(ops)+1)
	cPos := make([]int, len(ops)+1)
	for i, o := range ops {
		aPos[i+1], cPos[i+1] = aPos[i], cPos[i]
		if o.kind != opIns {
			aPos[i+1]++
		}
		if o.kind != opDel {
			cPos[i+1]++
		}
	}
	prevEnd := 0
	for k, g := range groups {
		lo := max(g.start-context, prevEnd)
		nextStart := len(ops)
		if k+1 < len(groups) {
			nextStart = groups[k+1].start
		}
		hi := min(g.end+context, nextStart)
		prevEnd = hi
		oldCount := aPos[hi] - aPos[lo]
		newCount := cPos[hi] - cPos[lo]
		fmt.Fprintf(b, "@@ -%s +%s @@\n", span(aPos[lo], oldCount), span(cPos[lo], newCount))
		for _, o := range ops[lo:hi] {
			b.WriteByte(byte(o.kind))
			b.WriteString(o.line)
			if !strings.HasSuffix(o.line, "\n") {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
}

// span is a hunk range the way diff prints it: "start,count", "start" for one
// line, and for no lines the line before them with a count of 0.
func span(before, count int) string {
	switch count {
	case 0:
		return fmt.Sprintf("%d,0", before)
	case 1:
		return fmt.Sprintf("%d", before+1)
	}
	return fmt.Sprintf("%d,%d", before+1, count)
}
