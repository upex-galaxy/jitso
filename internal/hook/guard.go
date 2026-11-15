package hook

import (
	"encoding/json"
	"regexp"
	"strings"
)

var orcaTool = regexp.MustCompile(`^mcp__.*orca.*__`)

// GuardVerb is the reason a PreToolUse payload launches an agent outside
// jitso, "" when it does not (the guard then stays silent). Bash commands are
// judged segment by segment; the tools of an MCP server whose name contains
// orca by tool name and arguments. Every other tool, and any field of the
// wrong type, passes: the guard protects a workflow and must never break an
// unrelated session.
func (l Launcher) GuardVerb(input map[string]any) string {
	name, _ := text(input["tool_name"])
	if name == "Bash" {
		command, ok := text(field(input["tool_input"], "command"))
		if !ok {
			return ""
		}
		return l.BashVerb(command)
	}
	return mcpVerb(name, input["tool_input"])
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

// LaunchWord is the command another session starts jitso launches with in a
// project: `bun run jitso launch` when its package.json declares a `jitso`
// script (the binary is not on PATH there), else `jitso launch`. It is the
// launcher word of the deny reason; the cases pin only the "(denied: ...)"
// tail.
func LaunchWord(packageJSON []byte) string {
	var pkg struct {
		Scripts map[string]any `json:"scripts"`
	}
	if json.Unmarshal(packageJSON, &pkg) == nil {
		if _, ok := pkg.Scripts["jitso"].(string); ok {
			return "bun run jitso launch"
		}
	}
	return "jitso launch"
}

// GuardReason is the deny text: it tells the model how to launch through
// jitso and names the verb that was denied.
func GuardReason(launchWord, verb string) string {
	return "JITSO owns worker launch in this project. Use: " + launchWord +
		" --build <id> --brief <file> --mode fleet --run <run> --task <task> [--worktree <sel>]." +
		" The orca-orchestration launch step is replaced; everything else in that skill still applies." +
		" (denied: " + verb + ")"
}
