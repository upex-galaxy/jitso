package doctor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/instructions"
	"github.com/upex-galaxy/jitso/internal/project"
)

// instructionsEnv is what the target lists read, over the doctor's ports.
func instructionsEnv(p Ports) instructions.Env {
	return instructions.Env{
		ReadFile: p.ReadFile,
		IsDir: func(path string) bool {
			info, err := p.Stat(path)
			return err == nil && info.IsDir()
		},
		EvalSymlinks: p.EvalSymlinks,
	}
}

// blockProblem is the broken-marker sentence of a file without the refusal
// ending the writing command adds.
func blockProblem(display string, err error) string {
	var pr *instructions.Problem
	if errors.As(err, &pr) {
		return strings.TrimSuffix(pr.Message(display), "; nothing written")
	}
	return display + ": " + message(err)
}

// checkInstructions looks for the JITSO block in the files `jitso setup
// instructions` examines for this checkout: it belongs in AGENTS.md. A
// CLAUDE.md or .claude/CLAUDE.md that does not import AGENTS.md (and is not
// the same file) is a warning, because Claude Code then reads only it; a
// block an older jitso left in one that imports AGENTS.md is a warning too.
func checkInstructions(p Ports, root project.Root) Line {
	const check = "instructions"
	env := instructionsEnv(p)
	env.Root = root.Checkout
	targets, err := instructions.ProjectTargets(env, true)
	if err != nil {
		return Line{Fail, check, message(err)}
	}

	var have, warns, fails []string
	number := 0
	for _, t := range targets.List {
		if t.Old == nil {
			warns = append(warns, "no "+t.Display+", so no JITSO block (run jitso setup instructions)")
			continue
		}
		b, err := instructions.Find(t.Old)
		switch {
		case err != nil:
			fails = append(fails, blockProblem(t.Display, err))
		case t.Legacy && !instructions.Imports(t.Old, t.Import):
			warns = append(warns, t.Display+" does not import AGENTS.md, so Claude Code does not read the JITSO block: add a line @"+t.Import+" to it")
		case t.Legacy && b != nil:
			warns = append(warns, leftInClaude(t.Display))
		case t.Legacy:
		case b == nil:
			warns = append(warns, "no JITSO block in "+t.Display+" (run jitso setup instructions)")
		case b.Number < instructions.Number:
			warns = append(warns, olderBlock(t.Display, b.Number))
		default:
			if len(have) == 0 {
				number = b.Number
			}
			have = append(have, t.Display)
		}
	}
	for _, w := range targets.Warnings {
		warns = append(warns, strings.TrimPrefix(w, "jitso: warning: "))
	}
	return instructionsLine(check, number, have, warns, fails)
}

// leftInClaude is the warning for a block an older jitso put in a project
// Claude Code memory file that imports AGENTS.md, which jitso no longer
// writes; setup instructions moves it.
func leftInClaude(display string) string {
	return display + " holds a JITSO block: run jitso setup instructions to move it to AGENTS.md"
}

// olderBlock is the warning for a block that a newer jitso replaces.
func olderBlock(display string, n int) string {
	return fmt.Sprintf("block %d in %s is older than block %d (run jitso setup instructions)", n, display, instructions.Number)
}

// checkInstructionsUser is the same look at the user's own files, the
// AGENTS.md of each harness user folder: it is optional, so no block anywhere
// is fine. A block in ~/.claude/CLAUDE.md is the one Claude Code reads for
// the user, so only its broken markers count.
func checkInstructionsUser(p Ports) Line {
	const check = "instructions-user"
	if p.Home == "" {
		return Line{Warn, check, "HOME is not set, so there are no user instruction files"}
	}
	env := instructionsEnv(p)
	env.Home, env.CodexHome = p.Home, p.Getenv("CODEX_HOME")
	targets, err := instructions.UserTargets(env, true)
	if err != nil {
		return Line{Fail, check, message(err)}
	}

	var have, warns, fails []string
	number := 0
	for _, t := range targets.List {
		if t.Old == nil {
			continue
		}
		b, err := instructions.Find(t.Old)
		switch {
		case err != nil:
			fails = append(fails, blockProblem(t.Display, err))
		case t.Legacy:
		case b == nil:
		case b.Number < instructions.Number:
			warns = append(warns, olderBlock(t.Display, b.Number))
		default:
			if len(have) == 0 {
				number = b.Number
			}
			have = append(have, t.Display)
		}
	}
	if len(have) == 0 && len(warns) == 0 && len(fails) == 0 {
		return Line{OK, check, "not set up (optional)"}
	}
	return instructionsLine(check, number, have, warns, fails)
}

// instructionsLine is the verdict of a list of files: any broken marker fails,
// any warning warns, else it is the block number and the files that have it.
func instructionsLine(check string, number int, have, warns, fails []string) Line {
	switch {
	case len(fails) > 0:
		return Line{Fail, check, strings.Join(fails, "; ")}
	case len(warns) > 0:
		return Line{Warn, check, strings.Join(warns, "; ")}
	}
	return Line{OK, check, "block " + strconv.Itoa(number) + " in " + strings.Join(have, ", ")}
}
