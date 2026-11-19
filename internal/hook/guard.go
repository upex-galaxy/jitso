package hook

import (
	"regexp"
	"strings"

	"github.com/upex-galaxy/jitso/internal/card"
)

var orcaTool = regexp.MustCompile(`^mcp__.*orca.*__`)

// Guard is what the guard refuses in a PreToolUse payload: a launch of an
// agent outside jitso (an orca launch verb, or a harness run as a program of
// its own), a `jitso build save` (the owner saves a draft as a build, an agent never does) or a `jitso setup` or `jitso install` that is not
// a dry run (the owner changes the setup), the zero Denial when it refuses nothing (the
// guard then stays silent). Bash commands are judged segment by segment; the
// tools of an MCP server whose name contains orca by tool name and arguments.
// Every other tool, and any field of the wrong type, passes: the guard protects
// a workflow and must never break an unrelated session.
func (l Launcher) Guard(input map[string]any) Denial {
	name, _ := text(input["tool_name"])
	if name == "Bash" {
		command, ok := text(field(input["tool_input"], "command"))
		if !ok {
			return Denial{}
		}
		return l.BashDenial(command)
	}
	return Denial{Verb: mcpVerb(name, input["tool_input"])}
}

// GuardVerb is the verb of Guard, "" when the payload is allowed.
func (l Launcher) GuardVerb(input map[string]any) string {
	return l.Guard(input).Verb
}

// Reason is the deny text of the denial; packageJSON is the project's
// package.json (empty when it has none), which decides how the launcher is
// spelled.
func (d Denial) Reason(packageJSON []byte) string {
	if d.Verb == VerbBuildSave {
		return SaveReason(LauncherWord(packageJSON), d.SaveID)
	}
	if d.Verb == VerbSetup || d.Verb == VerbInstall {
		return AdoptionReason(d.Verb)
	}
	if directHarnesses[d.Verb] {
		return HarnessReason(LaunchWord(packageJSON), d.Verb)
	}
	return GuardReason(LaunchWord(packageJSON), d.Verb)
}

// mcpVerb judges one tool of an orca MCP server.
func mcpVerb(name string, args any) string {
	if !orcaTool.MatchString(name) {
		return ""
	}
	tool := name[strings.LastIndex(name, "__")+2:]
	switch tool {
	case "start_worker":
		return "orca-bridge start_worker"
	case "create_worktree":
		if truthy(field(args, "agent")) {
			return "orca-bridge create_worktree with an agent"
		}
	case "create_terminal":
		if agent, ok := text(field(args, "agent")); ok && harnesses[agent] {
			return "orca-bridge create_terminal with a harness"
		}
	}
	return ""
}

// truthy is JavaScript truthiness for a decoded JSON value.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	}
	return true
}

// LauncherWord is the command another session runs jitso with in a project:
// `bun run jitso` when its package.json declares a `jitso` script that is not
// blank (the binary is not on PATH there), else `jitso`. The card decides the
// same way (card.DeclaresJitsoScript).
func LauncherWord(packageJSON []byte) string {
	if card.DeclaresJitsoScript(packageJSON) {
		return "bun run jitso"
	}
	return "jitso"
}

// LaunchWord is the launch command of LauncherWord. It is the launcher word of
// the deny reason; the cases pin only the "(denied: ...)" tail.
func LaunchWord(packageJSON []byte) string {
	return LauncherWord(packageJSON) + " launch"
}

// GuardReason is the deny text: it tells the model how to launch through
// jitso and names the verb that was denied.
func GuardReason(launchWord, verb string) string {
	return "JITSO owns worker launch in this project. Use: " + launchWord +
		" --build <id> --brief <file> --mode fleet --run <run> [--worktree <sel>]." +
		" The orca-orchestration launch step is replaced; everything else in that skill still applies." +
		" (denied: " + verb + ")"
}

// SaveReason is the deny text for `jitso build save`: it tells the model that
// only the owner saves a draft, and names the verb that was denied.
func SaveReason(launcherWord, id string) string {
	return "JITSO: only the owner saves a draft as a build; ask the owner to run: " + launcherWord +
		" build save " + id + " (denied: " + VerbBuildSave + ")"
}

// AdoptionReason is the deny text for `jitso setup` and `jitso install`: it
// tells the model that only the owner changes the setup, and names the verb
// that was denied (VerbSetup or VerbInstall).
func AdoptionReason(verb string) string {
	return "JITSO: only the owner changes the JITSO setup; ask the owner to run it in a terminal (denied: " + verb + ")"
}

// HarnessReason is the deny text for a harness run as a program of its own
// (defect N-24): it says a harness starts only through the launcher, and how
// to get past a word that is only text. name is claude, codex or opencode.
func HarnessReason(launchWord, name string) string {
	return "JITSO owns worker launch in this project: a harness starts only through " + launchWord +
		" --build <id> --brief <file> --mode <mode>, never as a program of its own." +
		" If the word is text (a commit message, a heredoc line), put the text in a file and pass the file." +
		" (denied: " + name + ")"
}
