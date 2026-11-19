// Package launch is `jitso launch`: parse and check the arguments, resolve
// the build, check the brief, print the card, and for a real launch write the
// launch folder, open the Orca terminal, answer the trust dialog, adopt the
// worker in fleet mode and append the launch event.
//
// Exit codes come from the errors it returns (package fail): 2 for a
// rejection, 3 for an Orca or environment error. A launch that fails before
// its registry line removes its own launch folder.
package launch

import (
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/brief"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/profile"
	"github.com/upex-galaxy/jitso/internal/shell"
)

// The launch modes. No other value exists, on the command line or in the
// registry.
const (
	ModeDirect = "direct"
	ModeFleet  = "fleet"
)

// Args are the parsed arguments of `jitso launch`. An optional value is nil
// when its flag was not given, which is not the same as an empty value: an
// empty --run is given and fails its allowlist.
type Args struct {
	Build string
	Brief string
	// Mode is direct or fleet.
	Mode     string
	Run      *string
	Task     *string
	Parent   *string
	Worktree *string
	// MCPExclude are the --mcp-exclude values, in order, as given.
	MCPExclude []string
	Fields     profile.Fields
	// BriefLanguage is en (default), es or any.
	BriefLanguage string
	DryRun        bool
}

// valueFlags are the flags that take the next word as their value. A word
// that starts with -- is never a value: the flag is missing its value
// (defect N-26).
var valueFlags = []string{"build", "brief", "mode", "run", "task", "parent", "worktree", "mcp-exclude", "field", "brief-language"}

// Parse reads the words after `launch`. It rejects an unknown word, a flag
// without a value (none left, or the next word starts with --), a malformed --field, a missing --build or --brief, a bad
// --brief-language and a --mode other than direct or fleet, in that order.
// A later value of a flag replaces an earlier one; --mcp-exclude and --field
// accumulate.
func Parse(argv []string) (Args, error) {
	a := Args{}
	values := map[string]string{}
	for i := 0; i < len(argv); i++ {
		flag := argv[i]
		if flag == "--dry-run" {
			a.DryRun = true
			continue
		}
		name := ""
		if strings.HasPrefix(flag, "--") {
			name = flag[2:]
		}
		if !slices.Contains(valueFlags, name) {
			return Args{}, fail.Rejected(`unknown argument "` + flag + `"`)
		}
		if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "--") {
			return Args{}, fail.Rejected("--" + name + " needs a value")
		}
		i++
		value := argv[i]
		switch name {
		case "mcp-exclude":
			a.MCPExclude = append(a.MCPExclude, value)
		case "field":
			at := strings.Index(value, "=")
			if at < 1 {
				return Args{}, fail.Rejected("--field must look like name=value")
			}
			a.Fields.Set(value[:at], value[at+1:])
		default:
			values[name] = value
		}
	}
	var ok bool
	if a.Build, ok = values["build"]; !ok {
		return Args{}, fail.Rejected("--build is required")
	}
	if a.Brief, ok = values["brief"]; !ok {
		return Args{}, fail.Rejected("--brief is required")
	}
	a.BriefLanguage = brief.LanguageEnglish
	if lang, given := values["brief-language"]; given {
		a.BriefLanguage = lang
	}
	switch a.BriefLanguage {
	case brief.LanguageEnglish, brief.LanguageSpanish, brief.LanguageAny:
	default:
		return Args{}, fail.Rejected("--brief-language must be en, es or any")
	}
	if a.Mode = values["mode"]; a.Mode != ModeDirect && a.Mode != ModeFleet {
		return Args{}, fail.Rejected("--mode must be direct or fleet")
	}
	a.Run, a.Task, a.Parent, a.Worktree = optional(values, "run"), optional(values, "task"), optional(values, "parent"), optional(values, "worktree")
	return a, nil
}

func optional(values map[string]string, name string) *string {
	if v, ok := values[name]; ok {
		return &v
	}
	return nil
}

// Check runs the launcher field allowlists on every value a model controls
// besides the brief (build, run, task, parent, worktree, each mcp-exclude,
// in that order), then, in fleet mode, requires --run and rejects --task (JITSO creates the Task). It needs no
// project, so it runs before anything is read.
func (a Args) Check() error {
	type field struct {
		name  string
		value *string
	}
	checks := []field{{"build", &a.Build}, {"run", a.Run}, {"task", a.Task}, {"parent", a.Parent}, {"worktree", a.Worktree}}
	for i := range a.MCPExclude {
		checks = append(checks, field{"mcp-exclude", &a.MCPExclude[i]})
	}
	for _, c := range checks {
		if c.value == nil {
			continue
		}
		if reason := shell.CheckLaunchField(c.name, *c.value); reason != "" {
			return fail.Rejected(reason)
		}
	}
	if a.Mode == ModeFleet {
		if a.Task != nil {
			return fail.Rejected("--task is not accepted in fleet mode: JITSO creates the Task from the brief")
		}
		if a.Run == nil {
			return fail.Rejected("fleet mode requires --run")
		}
	}
	return nil
}
