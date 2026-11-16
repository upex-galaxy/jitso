// Package tree is the launch tree of `jitso tree`: the launches of the
// registry nested by parent, each with the rollups of its branch. It is pure:
// it reads folded launches and returns nodes and text, and never does I/O.
//
// The rollups are the oracle's own (cli/jitso/tree.ts buildTree), computed on
// read and never stored. Cost and tokens are summed over the branch; peak
// context and budget percentage are the maximum of the nodes, because each
// node is its own context window and a sum would mean nothing; rule breaks are
// counted; the verdict of a branch is the worst of its analyzed nodes
// (fail, then unknown, then success), or pending when none is analyzed. A node
// with no analyzed event adds zeros.
package tree

import (
	"math"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Verdict names, from the analyzed event, and the one of a branch with no
// analyzed node.
const (
	VerdictPending = "pending"
	verdictSuccess = "success"
	verdictUnknown = "unknown"
	verdictFail    = "fail"
)

// severity orders the verdicts: the worst of a branch wins.
var severity = map[string]int{verdictSuccess: 0, verdictUnknown: 1, verdictFail: 2}

// Metrics are the numbers of a launch or of its whole branch.
type Metrics struct {
	CostUSDList float64
	// Tokens is input, cache read, cache write and output, summed.
	Tokens      float64
	PeakContext float64
	BudgetPct   float64
	RuleBreaks  int
	// Verdict is the worst verdict of the analyzed nodes, "" when none.
	Verdict string
	// Nodes counts the launches of the branch, this one included; Analyzed
	// counts the ones with an analyzed event.
	Nodes    int
	Analyzed int
}

// Node is a launch in the tree. Total covers the node and every descendant
// that is printed under it.
type Node struct {
	ID       string
	Launch   *registry.Launch
	Total    Metrics
	Children []*Node
	Depth    int
}

// Build is the tree of launches, in registry order. With root empty it holds
// every root: a launch with no parent or whose parent is not in the log. A
// launch that a parent cycle leaves out of every root is added as a root too,
// so nothing in the log is hidden. With a root id it is that launch's branch
// alone, or nothing when the id is not in the log. A launch is never in the
// tree twice.
func Build(launches []registry.Launch, root string) []*Node {
	byID := map[string]*registry.Launch{}
	for i := range launches {
		if _, dup := byID[launches[i].ID]; !dup {
			byID[launches[i].ID] = &launches[i]
		}
	}
	children := map[string][]*registry.Launch{}
	for i := range launches {
		if parent, ok := parentOf(&launches[i]); ok && byID[parent] != nil {
			children[parent] = append(children[parent], &launches[i])
		}
	}

	seen := map[string]bool{}
	var build func(l *registry.Launch, depth int) *Node
	build = func(l *registry.Launch, depth int) *Node {
		seen[l.ID] = true
		n := &Node{ID: l.ID, Launch: l, Depth: depth, Total: own(l)}
		for _, child := range children[l.ID] {
			if seen[child.ID] {
				continue
			}
			c := build(child, depth+1)
			n.Children = append(n.Children, c)
			combine(&n.Total, c.Total)
		}
		return n
	}

	if root != "" {
		l := byID[root]
		if l == nil {
			return nil
		}
		return []*Node{build(l, 0)}
	}
	var roots []*Node
	for i := range launches {
		l := &launches[i]
		if parent, ok := parentOf(l); ok && byID[parent] != nil {
			continue
		}
		if !seen[l.ID] {
			roots = append(roots, build(l, 0))
		}
	}
	for i := range launches {
		if l := &launches[i]; !seen[l.ID] {
			roots = append(roots, build(l, 0))
		}
	}
	return roots
}

// parentOf is the parent id of a launch line, false for null or any other
// value.
func parentOf(l *registry.Launch) (string, bool) { return l.Launch.String("parent") }

// own is the metrics of one launch.
func own(l *registry.Launch) Metrics {
	m := Metrics{RuleBreaks: len(l.RuleBreaks), Nodes: 1}
	if l.Analyzed == nil {
		return m
	}
	m.Analyzed = 1
	m.CostUSDList, _ = l.Analyzed.Number("cost_usd_list")
	for _, key := range []string{"input", "cache_read", "cache_write", "output"} {
		n, _ := l.Analyzed.Number("tokens." + key)
		m.Tokens += n
	}
	m.PeakContext, _ = l.Analyzed.Number("peak_context")
	m.BudgetPct, _ = l.Analyzed.Number("budget_pct")
	if v, _ := l.Analyzed.String("verdict"); v != "" {
		if _, known := severity[v]; known {
			m.Verdict = v
		}
	}
	return m
}

// combine adds a descendant's branch to a total.
func combine(into *Metrics, other Metrics) {
	into.CostUSDList += other.CostUSDList
	into.Tokens += other.Tokens
	into.PeakContext = math.Max(into.PeakContext, other.PeakContext)
	into.BudgetPct = math.Max(into.BudgetPct, other.BudgetPct)
	into.RuleBreaks += other.RuleBreaks
	into.Verdict = worst(into.Verdict, other.Verdict)
	into.Nodes += other.Nodes
	into.Analyzed += other.Analyzed
}

// worst is the more severe of two verdicts; "" is no verdict.
func worst(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	case severity[b] > severity[a]:
		return b
	}
	return a
}

// Lines is one text line per node, in preorder, indented two spaces per depth:
//
//	<id> <build> <state>: verdict <verdict>, US$ <cost> list, <tokens> tokens, peak <peak>, budget <pct>%, <k> rule break(s), <a> of <n> analyzed
//
// <build> is the launch line's build key, "-" when it is not a string; <state>
// is ended or running; the cost is rounded to three decimals.
func Lines(nodes []*Node) []string {
	var out []string
	var walk func(n *Node)
	walk = func(n *Node) {
		out = append(out, line(n))
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return out
}

func line(n *Node) string {
	build, ok := n.Launch.Launch.String(buildKey)
	if !ok {
		build = "-"
	}
	state := "running"
	if n.Launch.Ended() {
		state = "ended"
	}
	verdict := n.Total.Verdict
	if verdict == "" {
		verdict = VerdictPending
	}
	t := n.Total
	return strings.Repeat("  ", n.Depth) + n.ID + " " + build + " " + state +
		": verdict " + verdict +
		", US$ " + jsconv.Number(round3(t.CostUSDList)) + " list" +
		", " + jsconv.Number(t.Tokens) + " tokens" +
		", peak " + jsconv.Number(t.PeakContext) +
		", budget " + jsconv.Number(t.BudgetPct) + "%" +
		", " + jsconv.Number(float64(t.RuleBreaks)) + " rule break(s)" +
		", " + jsconv.Number(float64(t.Analyzed)) + " of " + jsconv.Number(float64(t.Nodes)) + " analyzed"
}

// buildKey is the key of the build id in a launch line (schema v2).
const buildKey = "build"

// round3 rounds to three decimals the way JavaScript's Math.round does, as the
// analyzer does for the cost it writes.
func round3(x float64) float64 { return jsRound(x*1000) / 1000 }

func jsRound(x float64) float64 {
	f := math.Floor(x)
	if x-f >= 0.5 {
		return f + 1
	}
	return f
}
