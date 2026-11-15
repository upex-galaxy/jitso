package launch

import (
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/profile"
)

// The texts `jitso build validate` prints for the rules only a launch can
// see. Launch itself keeps its own texts and exit codes (a missing budget is
// an environment error there, a harness without an adapter a rejection).
const (
	budgetProblem = "budget must be a mapping with tokens and usd_list_cap"
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
// this order: the budget, the harness, then each listed subagent that is not
// approved. A subagent-tier build never launches alone, so the first two do
// not apply to it (the tier matrix of docs/decisions.md). cat resolves the
// subagent names with the same layered lookup as a launch.
//
// It is pure, and it answers what Run would reject or fail on for this
// build: the same predicates, in the order the problem lines list them.
func Problems(b *catalog.Build, cat *catalog.Catalog) []string {
	var problems []string
	if b.Tier != catalog.TierSubagent {
		if !b.Budget.Set {
			problems = append(problems, budgetProblem)
		}
		if _, name, ok := adapterOf(b); !ok {
			problems = append(problems, `harness "`+name+`" has no adapter`)
		}
	}
	return append(problems, profile.LaunchProblems(b, cat)...)
}
