package instructions

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Target is one instruction file the command looks at.
type Target struct {
	// Path is the absolute path of the file to change: the real file when the
	// path a harness reads is a link (or lies in a linked folder), written
	// under the scope root. Display is the path the harness reads, as printed:
	// relative to the checkout for project scope, ~/... for user scope.
	Path    string
	Display string
	// Old is the content now; nil when the file does not exist.
	Old []byte
	// Legacy marks a Claude Code memory file (CLAUDE.md, .claude/CLAUDE.md,
	// ~/.claude/CLAUDE.md). jitso never writes a block there; an older jitso
	// did, so adding takes that block out of a project file that imports
	// AGENTS.md, and removing takes it out of every one.
	Legacy bool
	// Import is how a project Legacy file imports AGENTS.md of the checkout:
	// the path relative to that file, because Claude Code resolves an import
	// from the file that holds it ("" for the other files).
	Import string
	// PruneDir is a folder to remove, when it is empty, after this file is
	// deleted ("" for none); PruneDisplay is how it is printed. It is the
	// ~/.claude folder an older jitso made for a new ~/.claude/CLAUDE.md.
	PruneDir     string
	PruneDisplay string
}

// Targets is the file list of one scope, in plan order, and what to print
// about it.
type Targets struct {
	List []Target
	// Warnings go to stderr: a file jitso skips.
	Warnings []string
	// Notes are printed after the changes: a path that is the same file as
	// an earlier target, so it is changed once under the earlier name.
	Notes []string
	// None is why adding writes no AGENTS.md ("" when one is listed). The
	// add then changes nothing: an old block in a Claude file stays where it
	// is, because taking it out would leave the block nowhere.
	None string
}

// Env is what the target lists read from the machine. Tests give it
// in-memory answers.
type Env struct {
	// Root is the checkout for project scope.
	Root string
	// Home is the user's home folder for user scope.
	Home string
	// CodexHome is the value of $CODEX_HOME ("" when unset).
	CodexHome string
	// ReadFile returns the file's bytes, or an error wrapping fs.ErrNotExist
	// when it is absent.
	ReadFile func(path string) ([]byte, error)
	// IsDir reports whether path is a folder (a link to one counts).
	IsDir func(path string) bool
	// EvalSymlinks is filepath.EvalSymlinks; nil resolves nothing, so every
	// path is its own file.
	EvalSymlinks func(path string) (string, error)
}

func (e Env) read(path string) ([]byte, error) {
	data, err := e.ReadFile(path)
	switch {
	case err == nil:
		if data == nil {
			data = []byte{}
		}
		return data, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return nil, fail.Envf("could not read %s: %v", path, err)
}

// resolve is the file path stands for: path itself, or the real file when
// path or a folder on its way is a link, written under root so that it is
// still a path inside the scope root. inside is false when that real file
// lies outside root; real is then the resolved path.
func (e Env) resolve(root, path string) (real string, inside bool) {
	if e.EvalSymlinks == nil {
		return path, true
	}
	realRoot, err := e.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}
	real, err = e.EvalSymlinks(path)
	if err != nil {
		dir, derr := e.EvalSymlinks(filepath.Dir(path))
		if derr != nil {
			return path, true
		}
		real = filepath.Join(dir, filepath.Base(path))
	}
	rel, ok := relInside(realRoot, real)
	if !ok {
		return real, false
	}
	return filepath.Join(root, rel), true
}

// list builds a Targets: each file is resolved, a file outside the scope
// root is skipped with a warning, and a file that is the same as an earlier
// one is dropped with a note.
type list struct {
	env   Env
	root  string
	where string // the scope root in a warning: "this checkout", "your home folder"
	out   Targets
}

func (l *list) add(t Target) {
	path, inside := l.env.resolve(l.root, t.Path)
	if !inside {
		l.out.Warnings = append(l.out.Warnings, "jitso: warning: "+t.Display+" is a link to "+path+", which is outside "+l.where+", so jitso left it alone")
		return
	}
	for _, seen := range l.out.List {
		if seen.Path == path {
			l.out.Notes = append(l.out.Notes, "note: "+t.Display+" is the same file as "+seen.Display)
			return
		}
	}
	t.Path = path
	l.out.List = append(l.out.List, t)
}

// done ends the list of an add: with no AGENTS.md to write, nothing is
// listed and None says why.
func (l *list) done(remove bool, none string) Targets {
	if remove {
		return l.out
	}
	for _, t := range l.out.List {
		if !t.Legacy {
			return l.out
		}
	}
	l.out.List = nil
	l.out.None = none
	return l.out
}

// Project candidates, relative to the checkout.
const (
	agentsFile    = "AGENTS.md"
	claudeFile    = "CLAUDE.md"
	dotClaudeFile = ".claude/CLAUDE.md"
)

// noAgents is the None of an add whose every AGENTS.md was skipped with a
// warning.
const noAgents = "no AGENTS.md jitso may write"

// claudeImports is the Import of each project Legacy file.
var claudeImports = map[string]string{
	claudeFile:    "AGENTS.md",
	dotClaudeFile: "../AGENTS.md",
}

// ProjectTargets is the file list of project scope, in plan order: AGENTS.md
// of the checkout (listed absent or not; adding creates it), then each of
// CLAUDE.md and .claude/CLAUDE.md that exists, as a Legacy file.
//
// Claude Code reads only CLAUDE.md (or .claude/CLAUDE.md) when one exists, so
// adding lists a Claude file only when it imports AGENTS.md or is the same
// file: adding writes the block to AGENTS.md and takes an old one out of the
// Claude files listed. Any other Claude file is left alone with a warning
// naming the import line, and its old block stays. Removing lists every one.
func ProjectTargets(e Env, remove bool) (Targets, error) {
	l := list{env: e, root: e.Root, where: "this checkout"}
	for _, rel := range []string{agentsFile, claudeFile, dotClaudeFile} {
		path := filepath.Join(e.Root, filepath.FromSlash(rel))
		old, err := e.read(path)
		if err != nil {
			return Targets{}, err
		}
		legacy := rel != agentsFile
		if legacy && old == nil {
			continue
		}
		imp := claudeImports[rel]
		if legacy && !remove && !l.sameFile(path, filepath.Join(e.Root, agentsFile)) && !Imports(old, imp) {
			// jitso never writes a Claude file, so it says how to make
			// Claude Code read AGENTS.md, and an old block stays where it
			// is until then.
			head := "jitso: warning: " + rel + " does not import AGENTS.md, and Claude Code reads only " + rel + " when it exists: add a line @" + imp + " to it"
			if b, err := Find(old); err == nil && b != nil {
				l.out.Warnings = append(l.out.Warnings, head+"; its old JITSO block stays until then")
			} else {
				l.out.Warnings = append(l.out.Warnings, head+" so Claude Code reads the JITSO block")
			}
			continue
		}
		l.add(Target{Path: path, Display: rel, Old: old, Legacy: legacy, Import: imp})
	}
	return l.done(remove, noAgents), nil
}

// sameFile reports whether a and b resolve to one real file.
func (l *list) sameFile(a, b string) bool {
	ra, _ := l.env.resolve(l.root, a)
	rb, _ := l.env.resolve(l.root, b)
	return ra == rb
}

// UserTargets is the file list of user scope, in plan order.
//
// To add the block: AGENTS.md in each harness user folder that exists,
// created when absent: $CODEX_HOME (else ~/.codex), ~/.config/opencode and
// ~/.pi/agent. A Codex folder is skipped with a warning when its
// AGENTS.override.md has text, because Codex reads the override instead, and
// when it lies outside the home folder, because every file jitso writes lies
// under the scope root. ~/.config/opencode/AGENTS.md is not created, with a
// warning, while ~/.claude/CLAUDE.md exists, because OpenCode reads that file
// only while its own AGENTS.md is absent. ~/.claude/CLAUDE.md is not listed:
// it is what Claude Code reads for the user, so an old block there stays.
// With no harness folder, None says which folders were looked for.
//
// To remove it: the Codex file (the override or not) when its folder exists,
// the OpenCode and Pi files when they exist, and ~/.claude/CLAUDE.md always,
// as a Legacy file that prunes ~/.claude.
func UserTargets(e Env, remove bool) (Targets, error) {
	l := list{env: e, root: e.Home, where: "your home folder"}
	found := false

	codexDir := e.CodexHome
	if codexDir == "" {
		codexDir = filepath.Join(e.Home, ".codex")
	}
	codexDisplay, inside := homeDisplay(e.Home, codexDir)
	looked := []string{codexDisplay, "~/.config/opencode", "~/.pi/agent"}
	if e.IsDir(codexDir) {
		found = true
		switch {
		case !inside:
			l.out.Warnings = append(l.out.Warnings, "jitso: warning: CODEX_HOME ("+codexDir+") is not inside your home folder, so the JITSO block was not written to its AGENTS.md")
		default:
			path := filepath.Join(codexDir, "AGENTS.md")
			override, err := e.read(filepath.Join(codexDir, "AGENTS.override.md"))
			if err != nil {
				return Targets{}, err
			}
			cur, err := e.read(path)
			if err != nil {
				return Targets{}, err
			}
			if !remove && len(bytes.TrimSpace(override)) > 0 {
				l.out.Warnings = append(l.out.Warnings, "jitso: warning: "+codexDisplay+"/AGENTS.override.md is not empty, so Codex reads it instead of AGENTS.md; the JITSO block was not written to "+codexDisplay+"/AGENTS.md")
				break
			}
			l.add(Target{Path: path, Display: codexDisplay + "/AGENTS.md", Old: cur})
		}
	}

	claudeDir := filepath.Join(e.Home, ".claude")
	claude := filepath.Join(claudeDir, "CLAUDE.md")
	old, err := e.read(claude)
	if err != nil {
		return Targets{}, err
	}

	for _, rel := range []string{".config/opencode", ".pi/agent"} {
		dir := filepath.Join(e.Home, filepath.FromSlash(rel))
		if !e.IsDir(dir) {
			continue
		}
		found = true
		path := filepath.Join(dir, "AGENTS.md")
		cur, err := e.read(path)
		if err != nil {
			return Targets{}, err
		}
		if remove && cur == nil {
			continue
		}
		if !remove && cur == nil && rel == ".config/opencode" && old != nil {
			// OpenCode reads ~/.claude/CLAUDE.md only while its own
			// AGENTS.md is absent: creating it would hide those rules.
			l.out.Warnings = append(l.out.Warnings, "jitso: warning: ~/.config/opencode/AGENTS.md was not created, because OpenCode reads ~/.claude/CLAUDE.md only while it is absent; create it yourself (it may import your rules) and run again")
			continue
		}
		l.add(Target{Path: path, Display: "~/" + rel + "/AGENTS.md", Old: cur})
	}

	// ~/.claude/CLAUDE.md is what Claude Code reads for the user: adding
	// leaves an old block there; only --remove takes it out.
	if remove {
		l.add(Target{
			Path: claude, Display: "~/.claude/CLAUDE.md", Old: old, Legacy: true,
			PruneDir: claudeDir, PruneDisplay: "~/.claude",
		})
	}

	none := noAgents
	if !found {
		none = "no harness user folder found: " + strings.Join(looked, ", ")
	}
	return l.done(remove, none), nil
}

// homeDisplay is path written ~/... when it lies inside home.
func homeDisplay(home, path string) (string, bool) {
	rel, ok := relInside(home, path)
	if !ok {
		return path, false
	}
	return "~/" + filepath.ToSlash(rel), true
}

// relInside is path relative to root when it lies strictly inside root.
func relInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}
