package fakes

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jitso-contract-runner/internal/expect"
)

// Claude is the fake `claude`: it writes its argv, real cwd and whole environment to
// JITSO_CONTRACT_CLAUDE_DUMP and exits 0, printing nothing. It starts nothing.
// The one thing it answers is `claude mcp list`: it still writes its dump, then
// prints the scenario's mcp_list text on stdout and exits with mcp_list_exit.
func Claude(args []string, environ []string, stdout, stderr io.Writer) int {
	return Harness("claude", args, environ, stdout, stderr)
}

// Harness is the fake of one harness program (claude, codex, opencode or pi): the dump
// of the fake claude, with program set to the name it was started as (SPEC 5.2).
func Harness(program string, args []string, environ []string, stdout, stderr io.Writer) int {
	env := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	path := env[EnvClaudeDump]
	if path == "" {
		fmt.Fprintf(stderr, "fake %s: %s is not set\n", program, EnvClaudeDump)
		return 1
	}
	cwd, err := os.Getwd()
	if err == nil {
		if real, err := filepath.EvalSymlinks(cwd); err == nil {
			cwd = real
		}
	}
	if args == nil {
		args = []string{}
	}
	data, err := json.Marshal(expect.ClaudeDump{Program: program, Argv: args, Cwd: cwd, Env: env})
	if err != nil {
		fmt.Fprintln(stderr, "fake "+program+":", err)
		return 1
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintln(stderr, "fake "+program+":", err)
		return 1
	}
	cs := claudeKnobs(env[EnvScenario])
	if program == "claude" && len(args) == 2 && args[0] == "mcp" && args[1] == "list" {
		fmt.Fprint(stdout, cs.MCPList)
		return cs.MCPListExit
	}
	return cs.Exit
}

// claudeKnobs is the `claude` field of the scenario file; the zero value when
// there is none.
func claudeKnobs(scenarioPath string) ClaudeScenario {
	if scenarioPath == "" {
		return ClaudeScenario{}
	}
	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		return ClaudeScenario{}
	}
	var sc Scenario
	if json.Unmarshal(data, &sc) != nil {
		return ClaudeScenario{}
	}
	return sc.Claude
}
