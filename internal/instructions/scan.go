package instructions

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// line is one line of a file: the offsets of its first byte, of the end of its
// text (before the line break) and of the next line, and the text without the
// break. A last line with no break has next == end.
type line struct {
	num   int
	start int
	end   int
	next  int
	text  string
}

// splitLines cuts data into lines. A break is "\n" or "\r\n"; a "\r" that is
// not followed by "\n" stays in the text.
func splitLines(data []byte) []line {
	var out []line
	pos, num := 0, 0
	for pos < len(data) {
		num++
		i := bytes.IndexByte(data[pos:], '\n')
		if i < 0 {
			out = append(out, line{num: num, start: pos, end: len(data), next: len(data), text: string(data[pos:])})
			break
		}
		nl := pos + i
		end := nl
		if end > pos && data[end-1] == '\r' {
			end--
		}
		out = append(out, line{num: num, start: pos, end: end, next: nl + 1, text: string(data[pos:end])})
		pos = nl + 1
	}
	return out
}

// fences follows fenced code blocks (``` and ~~~) line by line, the way
// CommonMark does: an opening fence is up to three spaces, a run of three or
// more backticks or tildes (a backtick fence's info text holds no backtick);
// it closes with a run of the same character at least as long, and nothing
// but blanks after it. A fence not closed runs to the end of the file.
type fences struct {
	open bool
	ch   byte
	n    int
	from int // line number of the opening fence
}

// step reads one line and reports whether it is a fence line or inside a fence.
func (f *fences) step(l line) bool {
	ch, n, rest, ok := fenceRun(l.text)
	if f.open {
		if ok && ch == f.ch && n >= f.n && strings.TrimSpace(rest) == "" {
			f.open = false
		}
		return true
	}
	if ok && (ch == '~' || !strings.Contains(rest, "`")) {
		f.open, f.ch, f.n, f.from = true, ch, n, l.num
		return true
	}
	return false
}

// fenceRun reads a possible fence at the start of text: up to three spaces,
// then a run of at least three backticks or tildes.
func fenceRun(text string) (ch byte, n int, rest string, ok bool) {
	i := 0
	for i < len(text) && i < 4 && text[i] == ' ' {
		i++
	}
	if i > 3 || i >= len(text) || (text[i] != '`' && text[i] != '~') {
		return 0, 0, "", false
	}
	ch = text[i]
	j := i
	for j < len(text) && text[j] == ch {
		j++
	}
	if j-i < 3 {
		return 0, 0, "", false
	}
	return ch, j - i, text[j:], true
}

type markerKind int

const (
	markBegin markerKind = iota + 1
	markEnd
)

// marker is a BEGIN or END line found outside the fences.
type marker struct {
	kind  markerKind
	l     line
	block int
	added string
}

const (
	beginPrefix = "<!-- BEGIN JITSO"
	endPrefix   = "<!-- END JITSO"
)

var (
	reBlock = regexp.MustCompile(`(?:^|\s)block=(\d+)(?:\s|$)`)
	reAdded = regexp.MustCompile(`(?:^|\s)added=(new|[012])(?:\s|$)`)
)

// scanned is what scan finds in a file.
type scanned struct {
	markers []marker
	// openFence is the line number where a fence that the file ends inside
	// starts, 0 when the file ends outside any fence.
	openFence int
}

func scan(data []byte) scanned {
	var s scanned
	var f fences
	for _, l := range splitLines(data) {
		if f.step(l) {
			continue
		}
		switch {
		case strings.HasPrefix(l.text, beginPrefix):
			m := marker{kind: markBegin, l: l, added: Added0}
			head := strings.TrimPrefix(l.text, beginPrefix)
			if i := strings.IndexByte(head, ':'); i >= 0 {
				head = head[:i]
			}
			head = strings.TrimSuffix(strings.TrimSpace(head), "-->")
			if g := reBlock.FindStringSubmatch(head); g != nil {
				m.block, _ = strconv.Atoi(g[1])
			}
			if g := reAdded.FindStringSubmatch(head); g != nil {
				m.added = g[1]
			}
			s.markers = append(s.markers, m)
		case strings.HasPrefix(l.text, endPrefix):
			s.markers = append(s.markers, marker{kind: markEnd, l: l})
		}
	}
	if f.open {
		s.openFence = f.from
	}
	return s
}

// ProblemKind names what is wrong with a file's markers or fences.
type ProblemKind int

// The problems a file can have.
const (
	// BeginWithoutEnd: a BEGIN marker with no END after it.
	BeginWithoutEnd ProblemKind = iota + 1
	// EndWithoutBegin: an END marker with no BEGIN.
	EndWithoutBegin
	// EndBeforeBegin: an END marker that comes before the BEGIN it belongs to.
	EndBeforeBegin
	// SecondBlock: a second block after a complete one.
	SecondBlock
	// BeginInsideBlock: a BEGIN marker between a BEGIN and its END.
	BeginInsideBlock
	// OpenFence: the file ends inside a fenced code block, so a block added
	// to it would be quoted and never recognized.
	OpenFence
)

// Problem is a file jitso will not edit. It is an environment problem: the
// owner fixes the file by hand.
type Problem struct {
	Kind ProblemKind
	// Line is the 1-based line of the offending marker (of the opening fence
	// for OpenFence).
	Line int
}

func (p *Problem) what() string {
	switch p.Kind {
	case BeginWithoutEnd:
		return "a BEGIN without an END"
	case EndWithoutBegin:
		return "an END without a BEGIN"
	case EndBeforeBegin:
		return "an END before its BEGIN"
	case SecondBlock:
		return "a second block"
	case BeginInsideBlock:
		return "a BEGIN inside a block"
	}
	return "a broken marker"
}

// Error is the problem without a file name: "a BEGIN without an END at line 3".
func (p *Problem) Error() string {
	if p.Kind == OpenFence {
		return fmt.Sprintf("ends inside a code fence that starts at line %d", p.Line)
	}
	return fmt.Sprintf("%s at line %d", p.what(), p.Line)
}

// Message is the sentence that names the file, without the "jitso: " prefix:
//
//	<path> has a broken JITSO block (<problem> at line <n>); fix or remove the marker lines by hand; nothing written
//	<path> ends inside a code fence that starts at line <n>; close the fence by hand; nothing written
func (p *Problem) Message(path string) string {
	if p.Kind == OpenFence {
		return fmt.Sprintf("%s ends inside a code fence that starts at line %d; close the fence by hand; nothing written", path, p.Line)
	}
	return fmt.Sprintf("%s has a broken JITSO block (%s); fix or remove the marker lines by hand; nothing written", path, p.Error())
}

// Block is the block found in a file.
type Block struct {
	// Start is the offset of the first byte of the BEGIN line.
	Start int
	// End is the offset just after the text of the END line, before its break.
	End int
	// After is the offset of the line after the END line (past its break), or
	// the end of the file.
	After int
	// BeginLine is the 1-based line of the BEGIN marker.
	BeginLine int
	// Number is the `block=` value (0 when the BEGIN line has none).
	Number int
	// Added is the `added=` value: new, 0, 1 or 2 (0 when the BEGIN line has none).
	Added string
}

// Find returns the block of data, or nil when there is none. Markers inside
// fenced code blocks are ignored. A BEGIN without END, an END without BEGIN,
// an END before its BEGIN, two blocks and a BEGIN inside a block are a *Problem.
func Find(data []byte) (*Block, error) {
	return findIn(scan(data))
}

func findIn(s scanned) (*Block, error) {
	var open *marker
	var found *Block
	for i := range s.markers {
		m := s.markers[i]
		switch m.kind {
		case markBegin:
			if open != nil {
				return nil, &Problem{Kind: BeginInsideBlock, Line: m.l.num}
			}
			if found != nil {
				return nil, &Problem{Kind: SecondBlock, Line: m.l.num}
			}
			open = &s.markers[i]
		case markEnd:
			if open == nil {
				kind := EndWithoutBegin
				for _, later := range s.markers[i+1:] {
					if later.kind == markBegin {
						kind = EndBeforeBegin
						break
					}
				}
				return nil, &Problem{Kind: kind, Line: m.l.num}
			}
			found = &Block{
				Start:     open.l.start,
				End:       m.l.end,
				After:     m.l.next,
				BeginLine: open.l.num,
				Number:    open.block,
				Added:     open.added,
			}
			open = nil
		}
	}
	if open != nil {
		return nil, &Problem{Kind: BeginWithoutEnd, Line: open.l.num}
	}
	return found, nil
}
