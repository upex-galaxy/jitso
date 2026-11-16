package hook

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The areas the write boundary protects (defect N-25), as the deny text names
// them. The last one is the guard's own anchor: jitso/ itself.
const (
	areaBuilds = "jitso/builds/"
	areaRules  = "jitso/hard-rules/"
	areaYAML   = "jitso/jitso.yaml"
	areaRoot   = "jitso/"
)

// maxHops is how many symbolic links a path is followed through before it is
// given up (a loop the write would fail on too).
const maxHops = 40

// FS is what the path resolution asks of the file system: three calls the
// tests stand in for.
type FS struct {
	// LinkStat reports whether path exists (Lstat succeeds) and whether it is a
	// symbolic link.
	LinkStat func(path string) (exists, symlink bool)
	// Readlink is os.Readlink.
	Readlink func(path string) (string, error)
	// EvalSymlinks is filepath.EvalSymlinks.
	EvalSymlinks func(path string) (string, error)
}

// OSFS is the file system of this machine.
func OSFS() FS {
	return FS{
		LinkStat: func(path string) (bool, bool) {
			info, err := os.Lstat(path)
			return err == nil, err == nil && info.Mode()&os.ModeSymlink != 0
		},
		Readlink:     os.Readlink,
		EvalSymlinks: filepath.EvalSymlinks,
	}
}

// Boundary answers which writes the guard refuses: those into the approved
// builds (jitso/builds/), the hard rules (jitso/hard-rules/) and jitso.yaml
// (jitso/jitso.yaml) of any checkout, and the removal of jitso/ itself, since
// deleting jitso/jitso.yaml switches the whole guard off. The rule is on path
// components, so it holds for the primary, every linked worktree and the
// session's own checkout. Everything it touches outside its input is injected.
type Boundary struct {
	// Dir is the session's checkout (CLAUDE_PROJECT_DIR).
	Dir string
	// Home is $HOME, for ~ and $HOME in a path.
	Home string
	// Primary is the primary checkout, "" when it cannot be found. It is called
	// only when a removal operand needs it. May be nil.
	Primary func() string
	FS      FS
}

// Area is the protected area a path falls in by its components alone, "" when
// none: a component jitso (any letter case) followed by builds or hard-rules,
// or by jitso.yaml as the last component.
func Area(path string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for i := 0; i+1 < len(parts); i++ {
		if !strings.EqualFold(parts[i], "jitso") {
			continue
		}
		switch next := parts[i+1]; {
		case strings.EqualFold(next, "builds"):
			return areaBuilds
		case strings.EqualFold(next, "hard-rules"):
			return areaRules
		case strings.EqualFold(next, "jitso.yaml") && i+2 == len(parts):
			return areaYAML
		}
	}
	return ""
}

// lexicalArea is the check for a word whose folder is not known: it holds
// jitso/builds or jitso/hard-rules, or ends with jitso/jitso.yaml (quotes and
// backslashes removed, lowercased).
func lexicalArea(word string) string {
	w := strings.ToLower(strings.ReplaceAll(word, `\`, ""))
	switch {
	case strings.Contains(w, "jitso/builds"):
		return areaBuilds
	case strings.Contains(w, "jitso/hard-rules"):
		return areaRules
	case strings.HasSuffix(w, "jitso/jitso.yaml"):
		return areaYAML
	}
	return ""
}

var pathVar = regexp.MustCompile(`\$\{(HOME|PWD|CLAUDE_PROJECT_DIR)\}|\$(HOME|PWD|CLAUDE_PROJECT_DIR)\b`)

// expand rewrites a leading ~ and the variables a path may name, and says
// whether the word is now a plain path: no variable left, no glob character,
// and an absolute path or a known base folder.
func (b *Boundary) expand(text, base string, baseKnown bool) (string, bool) {
	t := strings.ReplaceAll(text, `\`, "")
	ok := true
	if t == "~" || strings.HasPrefix(t, "~/") {
		if b.Home == "" {
			ok = false
		}
		t = b.Home + t[1:]
	}
	t = pathVar.ReplaceAllStringFunc(t, func(m string) string {
		switch strings.Trim(m, "${}") {
		case "HOME":
			if b.Home == "" {
				ok = false
			}
			return b.Home
		case "PWD":
			if !baseKnown {
				ok = false
			}
			return base
		default:
			if b.Dir == "" {
				ok = false
			}
			return b.Dir
		}
	})
	if !ok || strings.Contains(t, "$") || strings.ContainsAny(t, "*?[{") {
		return "", false
	}
	if !filepath.IsAbs(t) && !baseKnown {
		return "", false
	}
	return t, true
}

// resolve follows the symbolic links a write to p would follow: a link at the
// end (even a dangling one), then the deepest existing ancestor with its own
// links resolved, and the missing tail appended. ok is false past maxHops.
func (b *Boundary) resolve(p string) (string, bool) {
	for hop := 0; hop < maxHops; hop++ {
		if exists, link := b.FS.LinkStat(p); exists && link {
			target, err := b.FS.Readlink(p)
			if err != nil {
				return p, true
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(p), target)
			}
			p = filepath.Clean(target)
			continue
		}
		existing, tail := p, ""
		for {
			if exists, _ := b.FS.LinkStat(existing); exists || existing == filepath.Dir(existing) {
				break
			}
			tail = filepath.Join(filepath.Base(existing), tail)
			existing = filepath.Dir(existing)
		}
		real, err := b.FS.EvalSymlinks(existing)
		if err != nil {
			return p, true
		}
		return filepath.Join(real, tail), true
	}
	return "", false
}

// coversRoot reports whether p is jitso/ of this checkout or of the primary, or
// a folder that holds it.
func (b *Boundary) coversRoot(p string) bool {
	targets := []string{}
	add := func(dir string) {
		if dir == "" {
			return
		}
		targets = append(targets, filepath.Join(dir, "jitso"))
		if real, err := b.FS.EvalSymlinks(dir); err == nil {
			targets = append(targets, filepath.Join(real, "jitso"))
		}
	}
	add(b.Dir)
	if b.Primary != nil {
		add(b.Primary())
	}
	low := strings.ToLower(filepath.Clean(p))
	for _, t := range targets {
		t = strings.ToLower(t)
		if t == low || low == "/" || strings.HasPrefix(t, strings.TrimSuffix(low, "/")+"/") {
			return true
		}
	}
	return false
}

// PathArea is the protected area a write to the path text names, "" when it is
// allowed. base is the folder a relative path is read from (known false when
// it cannot be told). removal adds the rule for the guard's own anchor: the
// path is jitso/ of a checkout or a folder that holds it. A path that cannot be
// resolved (a variable, a glob, an unknown base) gets the lexical check.
func (b *Boundary) PathArea(text, base string, baseKnown, removal bool) string {
	t, ok := b.expand(text, base, baseKnown)
	if !ok {
		return lexicalArea(text)
	}
	if !filepath.IsAbs(t) {
		t = filepath.Join(base, t)
	}
	lex := filepath.Clean(t)
	if area := Area(lex); area != "" {
		return area
	}
	resolved, ok := b.resolve(lex)
	if !ok {
		return ""
	}
	if area := Area(resolved); area != "" {
		return area
	}
	if removal && (b.coversRoot(lex) || b.coversRoot(resolved)) {
		return areaRoot
	}
	return ""
}

// toolPathField is the field of tool_input that names the file a file-writing
// tool changes, "" for any other tool.
func toolPathField(tool string) string {
	switch tool {
	case "Write", "Edit", "MultiEdit":
		return "file_path"
	case "NotebookEdit":
		return "notebook_path"
	}
	return ""
}

// ToolDenial judges a Write, Edit, MultiEdit or NotebookEdit call: input is
// the payload. The path is read from the folder in the payload's cwd, else
// Dir. A missing or mistyped field passes (the guard fails open).
func (b *Boundary) ToolDenial(input map[string]any) Denial {
	tool, _ := text(input["tool_name"])
	key := toolPathField(tool)
	if key == "" {
		return Denial{}
	}
	p, ok := text(field(input["tool_input"], key))
	if !ok || p == "" {
		return Denial{}
	}
	base, _ := text(input["cwd"])
	if base == "" {
		base = b.Dir
	}
	if area := b.PathArea(p, base, base != "", false); area != "" {
		return Denial{Verb: tool, Area: area}
	}
	return Denial{}
}

// shellState is the folder the next operand of a shell line is read from.
type shellState struct {
	base  string
	known bool
}

// cd follows a cd or pushd: the new folder, or unknown when the argument is
// missing, `-` or holds a variable the guard does not expand.
func (b *Boundary) cd(st *shellState, args []string) {
	var dir string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || a == "-" {
			dir = a
			break
		}
	}
	if dir == "" || dir == "-" {
		st.known = false
		return
	}
	t, ok := b.expand(dir, st.base, st.known)
	if !ok {
		st.known = false
		return
	}
	st.base, st.known = filepath.Join(st.base, t), true
	if filepath.IsAbs(t) {
		st.base = filepath.Clean(t)
	}
}

var redirectOp = regexp.MustCompile(`[0-9]*(?:<>|>>|>)`)

// redirectTargets are the files a segment's redirects write to: the text
// attached after the operator, else the next word. pending is true when the
// segment ends in an operator, so the target is the first word of the next
// segment (the splitter cuts at & and |).
func redirectTargets(words []string) (targets []string, pending bool) {
	for k, w := range words {
		locs := redirectOp.FindAllStringIndex(w, -1)
		for i, loc := range locs {
			end := len(w)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			switch t := w[loc[1]:end]; {
			case t != "":
				targets = append(targets, t)
			case k+1 < len(words):
				targets = append(targets, words[k+1])
			default:
				pending = true
			}
		}
	}
	return targets, pending
}

// operand is a word a writing program changes. removal marks one the program
// may delete or move away, which the anchor rule also judges.
type operand struct {
	text    string
	removal bool
}

// operands are the words of a program's arguments that are not options: a word
// after -- counts, the -- itself and a word starting with a dash do not.
func operands(args []string) []string {
	var out []string
	rest := false
	for _, a := range args {
		switch {
		case rest:
			out = append(out, a)
		case a == "--":
			rest = true
		case strings.HasPrefix(a, "-") && a != "-":
		default:
			out = append(out, a)
		}
	}
	return out
}

// flagValue is the value of a flag that takes one, as `-t dir`, `--long dir` or
// `--long=dir`.
func flagValue(args []string, short, long string) (string, bool) {
	for i, a := range args {
		switch {
		case a == short || a == long:
			if i+1 < len(args) {
				return args[i+1], true
			}
		case strings.HasPrefix(a, long+"="):
			return a[len(long)+1:], true
		}
	}
	return "", false
}

// hasShort reports whether an option cluster such as -sf holds the letter.
func hasShort(args []string, letters string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a[1:], letters) {
			return true
		}
	}
	return false
}

// inPlace reports whether the arguments ask a stream editor to edit its files
// in place: -i, -i.bak, -pi, --in-place, --inplace.
func inPlace(args []string) bool {
	for _, a := range args {
		switch {
		case a == "--in-place" || a == "--inplace" || strings.HasPrefix(a, "--in-place=") || strings.HasPrefix(a, "--inplace="):
			return true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--"):
			// a cluster of letters, then an optional suffix after the i
			for _, r := range a[1:] {
				if r == 'i' {
					return true
				}
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
					break
				}
			}
		}
	}
	return false
}

// writeOperands is what a program writes, by the table of the design: the verb
// the deny text names and the words it changes. verb is "" for a program that
// writes nothing the guard follows.
func writeOperands(prog string, args []string) (verb string, ops []operand) {
	plain := func(list []string, removal bool) {
		for _, w := range list {
			ops = append(ops, operand{w, removal})
		}
	}
	switch prog {
	case "tee":
		verb = "tee"
		plain(operands(args), false)
	case "cp", "install", "rsync":
		verb = prog
		all := operands(args)
		dir, hasDir := flagValue(args, "-t", "--target-directory")
		link := prog == "cp" && (hasShort(args, "ls") || flagIs(args, "--link", "--symbolic-link"))
		switch {
		case link:
			plain(all, false)
			if hasDir {
				plain([]string{dir}, false)
			}
		case hasDir:
			plain([]string{dir}, false)
		case len(all) > 0:
			plain(all[len(all)-1:], false)
		}
	case "ln":
		verb = "ln"
		plain(operands(args), false)
	case "mv":
		verb = "mv"
		all := operands(args)
		_, hasDir := flagValue(args, "-t", "--target-directory")
		for i, w := range all {
			ops = append(ops, operand{w, hasDir || i < len(all)-1})
		}
	case "rm", "rmdir":
		verb = prog
		plain(operands(args), true)
	case "unlink", "touch", "truncate", "shred":
		verb = prog
		plain(operands(args), false)
	case "sed", "perl", "yq":
		if !inPlace(args) {
			return "", nil
		}
		verb = prog + " -i"
		all := operands(args)
		switch prog {
		case "sed":
			if !hasShort(args, "ef") && !flagIs(args, "--expression", "--file") && len(all) > 0 {
				all = all[1:]
			}
		case "yq":
			if len(all) > 0 {
				all = all[1:]
			}
		case "perl":
			if v, ok := flagValue(args, "-e", "-E"); ok {
				all = removeFirst(all, v)
			}
		}
		plain(all, false)
	case "dd":
		verb = "dd"
		for _, a := range args {
			if v, ok := strings.CutPrefix(a, "of="); ok {
				plain([]string{v}, false)
			}
		}
	case "git":
		sub, rest := gitSubcommand(args)
		switch sub {
		case "mv", "rm", "restore", "checkout":
			verb = "git " + sub
			all := operands(rest)
			for i, w := range all {
				ops = append(ops, operand{w, sub == "rm" || sub == "mv" && i < len(all)-1})
			}
		}
	}
	return verb, ops
}

func flagIs(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n || strings.HasPrefix(a, n+"=") {
				return true
			}
		}
	}
	return false
}

func removeFirst(list []string, v string) []string {
	for i, w := range list {
		if w == v {
			return append(append([]string{}, list[:i]...), list[i+1:]...)
		}
	}
	return list
}

// gitSubcommand is the first word of git's arguments that is not an option
// (-C and -c take a value), and the words after it.
func gitSubcommand(args []string) (sub string, rest []string) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-C" || a == "-c":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a, args[i+1:]
		}
	}
	return "", nil
}

// judge is the denial of the first write in words (one segment, or the inner
// command of a shell -c), following a cd on the way; depth bounds the inner
// commands. A denial names what writes and which area.
func (b *Boundary) judge(words []string, st *shellState, depth int) Denial {
	if targets, _ := redirectTargets(words); len(targets) > 0 {
		for _, t := range targets {
			if area := b.PathArea(t, st.base, st.known, false); area != "" {
				return Denial{Verb: "redirect", Area: area}
			}
		}
	}
	prog, args, inner := Program(words)
	switch prog {
	case "cd", "pushd":
		b.cd(st, args)
	}
	verb, ops := writeOperands(prog, args)
	for _, op := range ops {
		if area := b.PathArea(op.text, st.base, st.known, op.removal); area != "" {
			return Denial{Verb: verb, Area: area}
		}
	}
	if depth < maxInnerDepth {
		for _, cmd := range inner {
			if d := b.judge(cmd, st, depth+1); d.Verb != "" {
				return d
			}
		}
	}
	return Denial{}
}

// BashWrites judges the segments of one Bash line in order, the way a shell
// runs them: it follows cd and pushd, and a segment that ends in a redirect
// operator takes the first word of the next one as its target (the splitter
// cuts at & and |).
type BashWrites struct {
	b       *Boundary
	st      shellState
	pending bool
}

// NewBashWrites starts a line that runs in the folder cwd ("" is Dir).
func (b *Boundary) NewBashWrites(cwd string) *BashWrites {
	if cwd == "" {
		cwd = b.Dir
	}
	return &BashWrites{b: b, st: shellState{base: cwd, known: cwd != ""}}
}

// Segment is the denial of the next segment, as its words, the zero Denial
// when it writes nowhere protected.
func (w *BashWrites) Segment(words []string) Denial {
	if w.pending {
		w.pending = false
		if area := w.b.PathArea(words[0], w.st.base, w.st.known, false); area != "" {
			return Denial{Verb: "redirect", Area: area}
		}
	}
	if d := w.b.judge(words, &w.st, 0); d.Verb != "" {
		return d
	}
	_, w.pending = redirectTargets(words)
	return Denial{}
}
