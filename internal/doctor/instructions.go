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
// instructions` writes for this checkout: AGENTS.md, and CLAUDE.md when Claude
// Code would read that instead and it does not import AGENTS.md.
func checkInstructions(p Ports, root project.Root) Line {
	const check = "instructions"
	env := instructionsEnv(p)
	env.Root = root.Checkout
	targets, warnings, err := instructions.ProjectTargets(env, false)
	if err != nil {
		return Line{Fail, check, message(err)}
	}

	var have, warns, fails []string
	number := 0
	for _, t := range targets {
		isAgents := t.Display == "AGENTS.md"
		if t.Old == nil {
			warns = append(warns, "no "+t.Display+", so no JITSO block (run jitso setup instructions)")
			continue
		}
		b, err := instructions.Find(t.Old)
		switch {
		case err != nil:
			fails = append(fails, blockProblem(t.Display, err))
		case b == nil && isAgents:
			warns = append(warns, "no JITSO block in AGENTS.md (run jitso setup instructions)")
		case b == nil:
			warns = append(warns, t.Display+" has no JITSO block and does not import @AGENTS.md (run jitso setup instructions)")
		case b.Number < instructions.Number:
			warns = append(warns, olderBlock(t.Display, b.Number))
		default:
			if len(have) == 0 {
				number = b.Number
			}
			have = append(have, t.Display)
		}
	}
	for _, w := range warnings {
		warns = append(warns, strings.TrimPrefix(w, "jitso: warning: "))
	}
	return instructionsLine(check, number, have, warns, fails)
}

// olderBlock is the warning for a block that a newer jitso replaces.
func olderBlock(display string, n int) string {
	return fmt.Sprintf("block %d in %s is older than block %d (run jitso setup instructions)", n, display, instructions.Number)
}

// checkInstructionsUser is the same look at the user's own files: it is
// optional, so no block anywhere is fine.
func checkInstructionsUser(p Ports) Line {
	const check = "instructions-user"
	if p.Home == "" {
		return Line{Warn, check, "HOME is not set, so there are no user instruction files"}
	}
	env := instructionsEnv(p)
	env.Home, env.CodexHome = p.Home, p.Getenv("CODEX_HOME")
	targets, _, err := instructions.UserTargets(env, false)
	if err != nil {
		return Line{Fail, check, message(err)}
	}

	var have, warns, fails []string
	number := 0
	for _, t := range targets {
		if t.Old == nil {
			continue
		}
		b, err := instructions.Find(t.Old)
		switch {
		case err != nil:
			fails = append(fails, blockProblem(t.Display, err))
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
