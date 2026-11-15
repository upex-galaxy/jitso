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
	// Path is the absolute path and Display the path as printed: relative to
	// the checkout for project scope, ~/... for user scope.
	Path    string
	Display string
	// Old is the content now; nil when the file does not exist.
	Old []byte
	// PruneDir is a folder to remove, when it is empty, after this file is
	// deleted ("" for none); PruneDisplay is how it is printed. It is the
	// ~/.claude folder jitso may have made for a new ~/.claude/CLAUDE.md.
	PruneDir     string
	PruneDisplay string
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

// Project candidates, relative to the checkout.
const (
	agentsFile      = "AGENTS.md"
	claudeFile      = "CLAUDE.md"
	dotClaudeFile   = ".claude/CLAUDE.md"
	claudeLocalFile = "CLAUDE.local.md"
)

// LocalWarning is printed (to stderr) when a CLAUDE.local.md that does not
// import AGENTS.md is the only Claude Code memory file next to an AGENTS.md.
const LocalWarning = "jitso: warning: CLAUDE.local.md stops Claude Code from reading AGENTS.md; add a line @AGENTS.md to it to keep the JITSO block"

// ProjectTargets is the file list of project scope, in plan order, and the
// warnings to print.
//
// To add the block (remove false): AGENTS.md (it is created when absent), then
// the first of CLAUDE.md and .claude/CLAUDE.md that exists, when it does not
// import AGENTS.md. Claude Code reads AGENTS.md only when none of CLAUDE.md,
// .claude/CLAUDE.md and CLAUDE.local.md exists, so a CLAUDE.local.md that does
// not import AGENTS.md, with no CLAUDE.md beside it, is a warning.
//
// To remove it (remove true): every one of the three files that exists, the
// import or not, so a CLAUDE.md that started importing AGENTS.md after the
// block was added is cleaned too. AGENTS.md is always listed, absent or not.
func ProjectTargets(e Env, remove bool) (targets []Target, warnings []string, err error) {
	mk := func(rel string) (Target, error) {
		path := filepath.Join(e.Root, filepath.FromSlash(rel))
		old, err := e.read(path)
		return Target{Path: path, Display: rel, Old: old}, err
	}
	agents, err := mk(agentsFile)
	if err != nil {
		return nil, nil, err
	}
	targets = append(targets, agents)

	var claude *Target
	for _, rel := range []string{claudeFile, dotClaudeFile} {
		t, err := mk(rel)
		if err != nil {
			return nil, nil, err
		}
		if t.Old == nil {
			continue
		}
		if remove {
			targets = append(targets, t)
			continue
		}
		if claude == nil {
			claude = &t
		}
	}
	if remove {
		return targets, nil, nil
	}
	if claude != nil {
		if !Imports(claude.Old) {
			targets = append(targets, *claude)
		}
		return targets, nil, nil
	}
	local, err := mk(claudeLocalFile)
	if err != nil {
		return nil, nil, err
	}
	if local.Old != nil && !Imports(local.Old) {
		warnings = append(warnings, LocalWarning)
	}
	return targets, warnings, nil
}

// UserTargets is the file list of user scope, in plan order, and the
// warnings to print.
//
// To add the block: ~/.claude/CLAUDE.md (created when absent);
// $CODEX_HOME/AGENTS.md (CODEX_HOME, else ~/.codex) only when that folder
// exists, and skipped with a warning when AGENTS.override.md there has text,
// because Codex reads the override instead; ~/.config/opencode/AGENTS.md only
// when it exists, because creating it would stop OpenCode from falling back to
// ~/.claude/CLAUDE.md. A Codex folder outside the home folder is skipped with a
// warning: every file jitso writes lies under the scope root.
//
// To remove it: the same three files, each only when it exists, the override
// or not.
func UserTargets(e Env, remove bool) (targets []Target, warnings []string, err error) {
	claudeDir := filepath.Join(e.Home, ".claude")
	claude := filepath.Join(claudeDir, "CLAUDE.md")
	old, err := e.read(claude)
	if err != nil {
		return nil, nil, err
	}
	targets = append(targets, Target{
		Path: claude, Display: "~/.claude/CLAUDE.md", Old: old,
		PruneDir: claudeDir, PruneDisplay: "~/.claude",
	})

	codexDir := e.CodexHome
	if codexDir == "" {
		codexDir = filepath.Join(e.Home, ".codex")
	}
	if e.IsDir(codexDir) {
		display, inside := homeDisplay(e.Home, codexDir)
		switch {
		case !inside:
			warnings = append(warnings, "jitso: warning: CODEX_HOME ("+codexDir+") is not inside your home folder, so the JITSO block was not written to its AGENTS.md")
		default:
			path := filepath.Join(codexDir, "AGENTS.md")
			override, err := e.read(filepath.Join(codexDir, "AGENTS.override.md"))
			if err != nil {
				return nil, nil, err
			}
			cur, err := e.read(path)
			if err != nil {
				return nil, nil, err
			}
			switch {
			case remove:
				targets = append(targets, Target{Path: path, Display: display + "/AGENTS.md", Old: cur})
			case len(bytes.TrimSpace(override)) > 0:
				warnings = append(warnings, "jitso: warning: "+display+"/AGENTS.override.md is not empty, so Codex reads it instead of AGENTS.md; the JITSO block was not written to "+display+"/AGENTS.md")
			default:
				targets = append(targets, Target{Path: path, Display: display + "/AGENTS.md", Old: cur})
			}
		}
	}

	opencode := filepath.Join(e.Home, ".config", "opencode", "AGENTS.md")
	cur, err := e.read(opencode)
	if err != nil {
		return nil, nil, err
	}
	if cur != nil {
		targets = append(targets, Target{Path: opencode, Display: "~/.config/opencode/AGENTS.md", Old: cur})
	}
	return targets, warnings, nil
}

// homeDisplay is path written ~/... when it lies inside home.
func homeDisplay(home, path string) (string, bool) {
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return path, false
	}
	return "~/" + filepath.ToSlash(rel), true
}
