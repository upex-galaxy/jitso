package launch

import (
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/profile"
)

// adapterOf is the adapter of the build's harness, the one lookup launch and
// validate share.
func adapterOf(b *catalog.Build) (harness.Adapter, string, bool) {
	name := jsString(b, "harness")
	a, ok := Adapters[name]
	return a, name, ok
}

// Problems are the launch rules a valid build breaks, as the plain problem
// lines `jitso build validate` prints after the load problems of its file, in
// this order: the harness (no adapter, or a field its adapter cannot honor,
// SupportProblems), then each listed subagent that is not approved.
// Launch itself keeps its own texts and exit codes (a harness without an
// adapter is a rejection there). A subagent-tier build never launches alone,
// so the harness rule does not apply to it (the tier matrix of
// docs/decisions.md). cat resolves the
// subagent names with the same lookup as a launch.
//
// It is pure, and it answers what Run would reject or fail on for this
// build: the same predicates, in the order the problem lines list them.
func Problems(b *catalog.Build, cat *catalog.Catalog) []string {
	var problems []string
	if b.Tier != catalog.TierSubagent {
		if a, name, ok := adapterOf(b); !ok {
			problems = append(problems, `harness "`+name+`" has no adapter`)
		} else {
			problems = append(problems, SupportProblems(b, a)...)
		}
	}
	return append(problems, profile.LaunchProblems(b, cat)...)
}

// SupportProblems are the fields of b that the harness of a cannot honor, one
// line each, naming the harness, in this order: subagents, tools, agents_md,
// skills_first (a user:<name> skill when the harness loads no skill from the
// launch package), mcp (any server when it takes none, else a user:<name>
// server when it takes no user server). An adapter never drops a field it
// cannot honor, so launch rejects these and build validate reports them.
func SupportProblems(b *catalog.Build, a harness.Adapter) []string {
	s := a.Support()
	h := `harness "` + a.ID() + `"`
	var problems []string
	if len(b.Subagents) > 0 && !s.Subagents {
		problems = append(problems, h+" cannot give a session subagents")
	}
	if b.ToolsSet && !s.Tools {
		problems = append(problems, h+" cannot limit the tools of a session")
	}
	if b.AgentsMD == "exclude" && !s.AgentsMDExclude {
		problems = append(problems, h+" cannot keep AGENTS.md out of a session")
	}
	if s.PackageFlag == "" {
		for _, item := range b.SkillsFirst {
			if _, ok := catalog.UserName(item); ok {
				problems = append(problems, h+` cannot load the user skill "`+item+`" from the launch package`)
			}
		}
	}
	switch {
	case !s.MCP && len(b.MCP) > 0:
		problems = append(problems, h+" cannot give a session MCP servers")
	case !s.UserMCP:
		for _, item := range b.MCP {
			if _, ok := catalog.UserName(item); ok {
				problems = append(problems, h+` cannot give a session the user mcp server "`+item+`"`)
			}
		}
	}
	return problems
}
