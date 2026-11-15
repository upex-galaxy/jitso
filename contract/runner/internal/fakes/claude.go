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
func Claude(args []string, environ []string, stderr io.Writer) int {
	env := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	path := env[EnvClaudeDump]
	if path == "" {
		fmt.Fprintf(stderr, "fake claude: %s is not set\n", EnvClaudeDump)
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
	data, err := json.Marshal(expect.ClaudeDump{Argv: args, Cwd: cwd, Env: env})
	if err != nil {
		fmt.Fprintln(stderr, "fake claude:", err)
		return 1
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintln(stderr, "fake claude:", err)
		return 1
	}
	return claudeExit(env[EnvScenario])
}

// claudeExit is the `claude.exit` knob of the scenario file; 0 when there is none.
func claudeExit(scenarioPath string) int {
	if scenarioPath == "" {
		return 0
	}
	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		return 0
	}
	var sc Scenario
	if json.Unmarshal(data, &sc) != nil {
		return 0
	}
	return sc.Claude.Exit
}
