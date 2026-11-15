package dispatch

import (
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/shell"
)

// The dispatcher is one fresh Haiku session per request.
const (
	Model  = "claude-haiku-5-5"
	Effort = "medium"
)

// CheckRequest is the request as the dispatcher takes it: trimmed, never
// empty, and never starting with a dash (it is the last argument of claude and
// must not pass for a flag). A bad request is a rejection (exit 2).
func CheckRequest(request string) (string, error) {
	text := jsconv.Trim(request)
	if text == "" {
		return "", fail.Rejected("the request is empty")
	}
	if strings.HasPrefix(text, "-") {
		return "", fail.Rejected("the request must not start with a dash")
	}
	return text, nil
}

// Plan is the claude line to run and the folder to run it in.
type Plan struct {
	Argv []string
	Dir  string
}

// PlanInput is everything the plan depends on.
type PlanInput struct {
	// Request is a checked request (CheckRequest).
	Request string
	// Template is the text of jitso/dispatcher/prompt.md.
	Template string
	// Summary is the catalog summary, one line per launchable build.
	Summary string
	// Launcher is the real path of this executable.
	Launcher string
	// DispatchDir is the fixed dispatcher folder.
	DispatchDir string
}

// BuildPlan is the interactive claude line: Haiku, project settings only, an
// empty strict MCP set, two tools, no permission bypass, the dispatch guard
// registered on Bash and Write, and the request as the last argument.
func BuildPlan(in PlanInput) (Plan, error) {
	briefs := BriefsDir(in.DispatchDir)
	prompt := RenderPrompt(in.Template, PromptParts{Catalog: in.Summary, Launcher: in.Launcher, BriefsDir: briefs})
	settings, err := Settings(in.Launcher)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Dir: in.DispatchDir,
		Argv: []string{
			"claude",
			"--model", Model,
			"--effort", Effort,
			"--setting-sources", "project",
			"--strict-mcp-config",
			"--mcp-config", `{"mcpServers":{}}`,
			// One `--tools=<list>` word: the flag is variadic and, as two words,
			// it would swallow the request that follows.
			"--tools=Bash,Write",
			"--settings", settings,
			"--system-prompt", prompt,
			in.Request,
		},
	}, nil
}

// BriefsDir is where the dispatcher writes the briefs it launches from.
func BriefsDir(dispatchDir string) string { return filepath.Join(dispatchDir, "briefs") }

// Settings is the --settings JSON: the dispatch guard, this binary's own
// `hook dispatch-guard`, as the only PreToolUse command hook, on Bash and
// Write.
func Settings(launcher string) (string, error) {
	quoted, err := shell.Quote(launcher)
	if err != nil {
		return "", fail.Envf("cannot register the dispatch guard: %v", err)
	}
	out, err := jsonx.Marshal(jsonx.Object{{Key: "hooks", Value: jsonx.Object{{Key: "PreToolUse", Value: []any{
		jsonx.Object{
			{Key: "matcher", Value: "Bash|Write"},
			{Key: "hooks", Value: []any{jsonx.Object{
				{Key: "type", Value: "command"},
				{Key: "command", Value: quoted + " hook dispatch-guard"},
			}}},
		},
	}}}}})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
