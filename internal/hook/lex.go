package hook

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// maxNest is how deep $( ), <( ), >( ) and backticks may nest before the line
// goes to the old splitter.
const maxNest = 64

// masked is the text a quoted word with whitespace becomes when the matchers
// read a command: a byte no matcher and no id pattern accepts.
const masked = "\x00"

// lexWord is one word of a lexed command.
type lexWord struct {
	// text is the word with its quotes removed; a substitution keeps its
	// source text, so the word still holds what the shell would run.
	text string
	// expand is true when the shell expands part of the word: a $ or a
	// backtick outside single quotes, or a process substitution.
	expand bool
}

// lexCommand is one simple command of a lexed line.
type lexCommand struct {
	words []lexWord
	start int
}

// heredoc is a here-document whose body is still to be read.
type heredoc struct {
	delim  string
	strip  bool
	quoted bool
}

type lexer struct {
	s       string
	pos     int
	base    int
	nest    int
	pending []heredoc
	out     *[]lexCommand
}

// lex reads a Bash line the way the shell splits it: words with their quotes
// removed, commands cut at ; & && | || newline ( and ) outside quotes, the
// commands of every $( ), backtick and process substitution as commands of
// their own, and heredoc bodies kept out of the commands (their substitutions
// are read when the delimiter is not quoted). ok is false when the line does
// not parse: an unbalanced quote, parenthesis or backtick, a heredoc without
// its delimiter line, or nesting deeper than maxNest.
func lex(line string) (cmds []lexCommand, ok bool) {
	lx := &lexer{s: line, out: &cmds}
	if !lx.list(false) || len(lx.pending) > 0 {
		return nil, false
	}
	sort.SliceStable(cmds, func(i, j int) bool { return cmds[i].start < cmds[j].start })
	return cmds, true
}

// sub lexes s, a part of the line at offset base, into the same commands.
func (lx *lexer) sub(s string, base int) bool {
	if lx.nest >= maxNest {
		return false
	}
	in := &lexer{s: s, base: base, nest: lx.nest + 1, out: lx.out}
	return in.list(false) && len(in.pending) == 0
}

// list reads commands up to the end of the string or, when inSub, up to the
// ) that closes a substitution (consumed).
func (lx *lexer) list(inSub bool) bool {
	var words []lexWord
	var b strings.Builder
	inWord, expand := false, false
	start := lx.pos
	depth := 0
	endWord := func() {
		if inWord && b.Len() > 0 {
			words = append(words, lexWord{text: b.String(), expand: expand})
		}
		b.Reset()
		inWord, expand = false, false
	}
	endCommand := func() {
		endWord()
		if len(words) > 0 {
			*lx.out = append(*lx.out, lexCommand{words: words, start: lx.base + start})
		}
		words = nil
		start = lx.pos
	}
	for lx.pos < len(lx.s) {
		c := lx.s[lx.pos]
		next := byte(0)
		if lx.pos+1 < len(lx.s) {
			next = lx.s[lx.pos+1]
		}
		switch {
		case c == '\n':
			endCommand()
			lx.pos++
			if !lx.bodies() {
				return false
			}
			start = lx.pos
		case c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f':
			endWord()
			lx.pos++
		case c == ';' || c == '|':
			endCommand()
			lx.pos++
		case c == '&':
			if inWord && b.Len() > 0 && (b.String()[b.Len()-1] == '>' || b.String()[b.Len()-1] == '<') {
				b.WriteByte(c)
				lx.pos++
				continue
			}
			endCommand()
			lx.pos++
		case c == '(':
			endCommand()
			depth++
			lx.pos++
		case c == ')':
			endCommand()
			lx.pos++
			switch {
			case depth > 0:
				depth--
			case inSub:
				return true
			default:
				return false
			}
		case c == '\'':
			end := strings.IndexByte(lx.s[lx.pos+1:], '\'')
			if end < 0 {
				return false
			}
			b.WriteString(lx.s[lx.pos+1 : lx.pos+1+end])
			inWord = true
			lx.pos += end + 2
		case c == '"':
			if !lx.double(&b, &expand) {
				return false
			}
			inWord = true
		case c == '\\':
			inWord = true
			switch {
			case next == '\n':
				lx.pos += 2
			case lx.pos+1 < len(lx.s):
				b.WriteByte(next)
				lx.pos += 2
			default:
				b.WriteByte(c)
				lx.pos++
			}
		case c == '`':
			if !lx.backtick(&b) {
				return false
			}
			inWord, expand = true, true
		case (c == '$' || c == '<' || c == '>') && next == '(':
			if !lx.substitution(&b) {
				return false
			}
			inWord, expand = true, true
		case c == '<' && strings.HasPrefix(lx.s[lx.pos:], "<<<"):
			endWord()
			words = append(words, lexWord{text: "<<<"})
			lx.pos += 3
		case c == '<' && next == '<':
			endWord()
			if !lx.heredocStart() {
				return false
			}
			words = append(words, lexWord{text: "<<"}, lexWord{text: lx.pending[len(lx.pending)-1].delim})
		case c == '$':
			b.WriteByte(c)
			inWord, expand = true, true
			lx.pos++
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRuneInString(lx.s[lx.pos:])
			if jsconv.IsSpace(r) {
				endWord()
			} else {
				b.WriteString(lx.s[lx.pos : lx.pos+size])
				inWord = true
			}
			lx.pos += size
		default:
			b.WriteByte(c)
			inWord = true
			lx.pos++
		}
	}
	endCommand()
	return !inSub && depth == 0
}

// double reads a double-quoted part from its opening quote, the way the shell
// does: \ escapes only $ ` " \ and a newline, and $( ) and backticks still run.
func (lx *lexer) double(b *strings.Builder, expand *bool) bool {
	lx.pos++
	for lx.pos < len(lx.s) {
		c := lx.s[lx.pos]
		next := byte(0)
		if lx.pos+1 < len(lx.s) {
			next = lx.s[lx.pos+1]
		}
		switch {
		case c == '"':
			lx.pos++
			return true
		case c == '\\' && (next == '$' || next == '`' || next == '"' || next == '\\'):
			b.WriteByte(next)
			lx.pos += 2
		case c == '\\' && next == '\n':
			lx.pos += 2
		case c == '`':
			if !lx.backtick(b) {
				return false
			}
			*expand = true
		case c == '$' && next == '(':
			if !lx.substitution(b) {
				return false
			}
			*expand = true
		case c == '$':
			b.WriteByte(c)
			*expand = true
			lx.pos++
		default:
			b.WriteByte(c)
			lx.pos++
		}
	}
	return false
}

// substitution reads $( ), <( ) or >( ) from its first byte: the commands
// inside are lexed with the rest, and the word keeps the source text.
func (lx *lexer) substitution(b *strings.Builder) bool {
	if lx.nest >= maxNest {
		return false
	}
	from := lx.pos
	lx.pos += 2
	lx.nest++
	ok := lx.list(true)
	lx.nest--
	if !ok {
		return false
	}
	b.WriteString(lx.s[from:lx.pos])
	return true
}

// backtick reads a backtick substitution from its opening backtick and lexes
// its text, with the \` \\ \$ escapes undone, as a line of its own.
func (lx *lexer) backtick(b *strings.Builder) bool {
	from := lx.pos
	var inner strings.Builder
	for i := from + 1; i < len(lx.s); i++ {
		switch c := lx.s[i]; {
		case c == '\\' && i+1 < len(lx.s):
			if n := lx.s[i+1]; n == '`' || n == '\\' || n == '$' {
				inner.WriteByte(n)
			} else {
				inner.WriteByte(c)
				inner.WriteByte(n)
			}
			i++
		case c == '`':
			if !lx.sub(inner.String(), lx.base+from+1) {
				return false
			}
			b.WriteString(lx.s[from : i+1])
			lx.pos = i + 1
			return true
		default:
			inner.WriteByte(c)
		}
	}
	return false
}

// heredocStart reads << or <<- and the delimiter word after it, and queues
// the body, which starts after the next newline.
func (lx *lexer) heredocStart() bool {
	lx.pos += 2
	h := heredoc{}
	if lx.pos < len(lx.s) && lx.s[lx.pos] == '-' {
		h.strip = true
		lx.pos++
	}
	for lx.pos < len(lx.s) && (lx.s[lx.pos] == ' ' || lx.s[lx.pos] == '\t') {
		lx.pos++
	}
	var d strings.Builder
	for lx.pos < len(lx.s) {
		c := lx.s[lx.pos]
		switch {
		case c == '\'' || c == '"':
			end := strings.IndexByte(lx.s[lx.pos+1:], c)
			if end < 0 {
				return false
			}
			d.WriteString(lx.s[lx.pos+1 : lx.pos+1+end])
			h.quoted = true
			lx.pos += end + 2
			continue
		case c == '\\' && lx.pos+1 < len(lx.s):
			d.WriteByte(lx.s[lx.pos+1])
			h.quoted = true
			lx.pos += 2
			continue
		case strings.IndexByte(" \t\n;&|()<>`$", c) >= 0:
		default:
			d.WriteByte(c)
			lx.pos++
			continue
		}
		break
	}
	if d.Len() == 0 {
		return false
	}
	h.delim = d.String()
	lx.pending = append(lx.pending, h)
	return true
}

// bodies reads the bodies of the queued heredocs, which start at the current
// position. A body is data; the substitutions of a body whose delimiter is not
// quoted are lexed, because the shell expands them.
func (lx *lexer) bodies() bool {
	for _, h := range lx.pending {
		from := lx.pos
		for {
			if lx.pos >= len(lx.s) {
				return false
			}
			line, next := lx.s[lx.pos:], len(lx.s)
			if eol := strings.IndexByte(line, '\n'); eol >= 0 {
				line, next = line[:eol], lx.pos+eol+1
			}
			if h.strip {
				line = strings.TrimLeft(line, "\t")
			}
			if line == h.delim {
				body := lx.s[from:lx.pos]
				lx.pos = next
				if !h.quoted && !lx.bodySubstitutions(body, lx.base+from) {
					return false
				}
				break
			}
			lx.pos = next
		}
	}
	lx.pending = nil
	return true
}

// bodySubstitutions lexes the $( ) and backtick substitutions of an unquoted
// heredoc body.
func (lx *lexer) bodySubstitutions(body string, base int) bool {
	if lx.nest >= maxNest {
		return false
	}
	in := &lexer{s: body, base: base, nest: lx.nest + 1, out: lx.out}
	var discard strings.Builder
	for in.pos < len(body) {
		c := body[in.pos]
		switch {
		case c == '\\':
			in.pos += 2
		case c == '$' && in.pos+1 < len(body) && body[in.pos+1] == '(':
			if !in.substitution(&discard) || len(in.pending) > 0 {
				return false
			}
		case c == '`':
			if !in.backtick(&discard) {
				return false
			}
		default:
			in.pos++
		}
		discard.Reset()
	}
	return true
}

// texts are the words of c as text.
func (c lexCommand) texts() []string {
	out := make([]string, len(c.words))
	for i, w := range c.words {
		out[i] = w.text
	}
	return out
}

// valueFlags are the flags of each data command whose value is text.
var valueFlags = map[string]map[string]bool{
	"git":  {"-m": true, "--message": true},
	"orca": {"--body": true, "--subject": true, "--question": true},
	"gh":   {"--body": true, "-b": true, "--title": true, "-t": true, "--notes": true, "-n": true},
}

// ghVerbs are the gh commands that only send text.
var ghVerbs = map[string]bool{"create": true, "edit": true, "comment": true}

// dataPositions are the indexes of the words of a data command that hold
// text: the values of its text flags, every argument of echo and printf. ok
// is false when the command is not a data command: only `git commit` and
// `git tag` (a heredoc only with -F - or --file=-), `orca orchestration
// send`, `ask` and `reply`, `gh pr`, `issue` and `release` `create`, `edit`
// and `comment`, echo and printf, and cat that reads a heredoc and writes to
// its output or a > or >> file. Any other program, a NAME=value word in front,
// a here-string, or a heredoc on another command makes the line one the
// guard judges with the old splitter.
func dataPositions(words []string) (data []int, ok bool) {
	heredoc := false
	for _, w := range words {
		switch w {
		case "<<<":
			return nil, false
		case "<<":
			heredoc = true
		}
	}
	switch prog := words[0]; {
	case prog == "echo" || prog == "printf":
		if heredoc {
			return nil, false
		}
		for i := 1; i < len(words); i++ {
			data = append(data, i)
		}
		return data, true
	case prog == "cat":
		return nil, heredoc && catRedirectsOnly(words[1:])
	case prog == "git" && len(words) > 1 && (words[1] == "commit" || words[1] == "tag"):
		stdin := false
		for _, w := range words[2:] {
			switch {
			case w == "-c" || w == "-C" || w == "--exec" || w == "-x" || w == "--config" || strings.HasPrefix(w, "--exec=") || strings.HasPrefix(w, "--config="):
				return nil, false
			case w == "--file=-":
				stdin = true
			}
		}
		for i := 2; i+1 < len(words); i++ {
			if (words[i] == "-F" || words[i] == "--file") && words[i+1] == "-" {
				stdin = true
			}
		}
		if heredoc && !stdin {
			return nil, false
		}
		return values(words, 2, valueFlags["git"], true), true
	case prog == "orca" && len(words) > 2 && words[1] == "orchestration" && (words[2] == "send" || words[2] == "ask" || words[2] == "reply"):
		if heredoc {
			return nil, false
		}
		return values(words, 3, valueFlags["orca"], false), true
	case prog == "gh" && len(words) > 2 && (words[1] == "pr" || words[1] == "issue" || words[1] == "release") && ghVerbs[words[2]]:
		if heredoc {
			return nil, false
		}
		return values(words, 3, valueFlags["gh"], false), true
	}
	return nil, false
}

// values are the indexes of the values of flags in words from from on: the
// word after a flag, or a --flag=value word. shortJoined also takes a group of
// short options that ends in m (git's -am) as the flag.
func values(words []string, from int, flags map[string]bool, shortJoined bool) []int {
	var out []int
	for i := from; i < len(words); i++ {
		w := words[i]
		if k := strings.IndexByte(w, '='); k > 0 && strings.HasPrefix(w, "--") && flags[w[:k]] {
			out = append(out, i)
			continue
		}
		joined := shortJoined && len(w) > 2 && w[0] == '-' && w[1] != '-' && w[len(w)-1] == 'm' && strings.Trim(w[1:], "abcdefghijklmnopqrstuvwxyz") == ""
		if (flags[w] || joined) && i+1 < len(words) {
			out = append(out, i+1)
			i++
		}
	}
	return out
}

// catRedirectsOnly reports whether the words after cat are only the heredoc
// and > or >> redirects to a file.
func catRedirectsOnly(words []string) bool {
	for i := 0; i < len(words); i++ {
		switch w := words[i]; {
		case w == "<<" || w == ">" || w == ">>":
			i++
		case strings.HasPrefix(w, ">>") || strings.HasPrefix(w, ">"):
		default:
			return false
		}
	}
	return true
}

// matchText is the command as the matchers read it: the words joined by one
// space, where a word in a data position that holds whitespace reads as
// masked.
func matchText(words []string, data []int) string {
	parts := append([]string(nil), words...)
	for _, k := range data {
		if strings.IndexFunc(parts[k], jsconv.IsSpace) >= 0 {
			parts[k] = masked
		}
	}
	return strings.Join(parts, " ")
}

// lexedDenial is the denial of command read with the lexer. ok is false when
// the line must be judged by the old splitter instead: it does not lex, or one
// of its commands (each part of a list or a pipeline, and each substitution)
// is not a data command (dataPositions).
func (l Launcher) lexedDenial(command string) (d Denial, ok bool) {
	cmds, ok := lex(command)
	if !ok {
		return Denial{}, false
	}
	texts := make([]string, len(cmds))
	for i, c := range cmds {
		words := c.texts()
		data, ok := dataPositions(words)
		if !ok {
			return Denial{}, false
		}
		texts[i] = matchText(words, data)
	}
	for _, text := range texts {
		if d := l.verdict(text); d.Verb != "" {
			return d, true
		}
	}
	return Denial{}, true
}

// verdict is the denial of one data command as the matchers read it. Its
// program is never a harness, so only the text matchers can deny it.
func (l Launcher) verdict(text string) Denial {
	if id, ok := l.BuildSave(text); ok {
		return Denial{Verb: VerbBuildSave, SaveID: id}
	}
	if verb, ok := l.Adoption(text); ok {
		return Denial{Verb: verb}
	}
	if verb := l.LaunchVerb(text); verb != "" {
		return Denial{Verb: verb}
	}
	return Denial{}
}
