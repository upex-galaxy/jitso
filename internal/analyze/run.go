package analyze

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Usage is the usage line of the command, in every rejection of its arguments.
const Usage = "usage: jitso analyze <launch_id> | jitso analyze --transcript <file.jsonl>"

// Deps is what Run needs from the outside: where the prices are, the registry,
// the catalog (read only for a launch's success checks) and the clock.
type Deps struct {
	// PricesPath is <primary checkout>/jitso/prices.json.
	PricesPath string
	Store      registry.Store
	// Catalog loads the project's catalog; it is called only by a by-id run.
	Catalog func() (*catalog.Catalog, error)
	Now     func() time.Time
}

// Request is what `jitso analyze` was asked: one launch id, or one transcript
// file to analyze without writing anything.
type Request struct {
	// ID is the launch id of `analyze <launch_id>`; "" for a transcript.
	ID string
	// Transcript is the path of `analyze --transcript <file.jsonl>`.
	Transcript string
}

// ParseArgs reads the arguments: one launch id of the exact form, or
// `--transcript` and one path that ends in .jsonl, with nothing after. Anything
// else is a rejection (exit 2). It needs no project and no price table, so it
// runs before either is looked at (TS-ODDITIES R6, defect N-28).
func ParseArgs(args []string) (Request, error) {
	if len(args) > 0 && args[0] == "--transcript" {
		if len(args) != 2 || !strings.HasSuffix(args[1], ".jsonl") {
			return Request{}, fail.Rejected(Usage)
		}
		return Request{Transcript: args[1]}, nil
	}
	if len(args) != 1 || !registry.IsLaunchID(args[0]) {
		return Request{}, fail.Rejected(Usage)
	}
	return Request{ID: args[0]}, nil
}

// Run is `jitso analyze` for a parsed request. It returns the text for stdout:
// the one-line summary of `analyze <launch_id>` (which also appends events) or
// the indented JSON of `analyze --transcript <file.jsonl>` (which writes
// nothing). The price table loads first: a missing prices file is exit 3, after
// the arguments and the project have passed.
func Run(req Request, d Deps) (string, error) {
	prices, err := LoadPrices(d.PricesPath)
	if err != nil {
		return "", err
	}
	if req.Transcript != "" {
		a, err := Session(req.Transcript, prices)
		if err != nil {
			return "", fail.Rejected(err.Error())
		}
		text, err := jsonx.MarshalIndent(a, " ")
		if err != nil {
			return "", err
		}
		return string(text), nil
	}
	return byID(req.ID, prices, d)
}

// verdictOf is the verdict of an analysis against the build's success checks:
// fail when a tool call touched a secret path or the launch went over its
// budget (both are breaks of a rule the analyzer sees, so a success next to
// them would be false), success when every check of the build is one that can
// be decided here (no_rule_breaks), otherwise unknown.
func verdictOf(secretReads int, overBudget bool, checks []catalog.SuccessCheck) (verdict, reason string) {
	evaluated := 0
	for _, c := range checks {
		if c.Check == "no_rule_breaks" {
			evaluated++
		}
	}
	if secretReads > 0 {
		return "fail", "a tool call read a secret path"
	}
	if overBudget {
		return "fail", "the launch went over its budget"
	}
	verdict = "unknown"
	if len(checks) > 0 && evaluated == len(checks) {
		verdict = "success"
	}
	return verdict, fmt.Sprintf("build success checks %d of %d evaluated here; the rest need the destination", evaluated, len(checks))
}

// budgetBreaks are the evidence texts of the limits the launch went over, the
// token budget first, then the list cost cap (TS-ODDITIES R10, defect N-28).
// A limit that is absent, not a number or not above zero is not checked. The
// budget is detected after the session, never enforced during it.
func budgetBreaks(l *registry.Launch, a Analysis) []string {
	var out []string
	if tokens, ok := l.Launch.Number("budget.tokens"); ok && tokens > 0 && a.MaxMainContextTokens > tokens {
		out = append(out, fmt.Sprintf("peak context %s tokens is over the budget of %s tokens",
			jsconv.Number(a.MaxMainContextTokens), jsconv.Number(tokens)))
	}
	if cap, ok := l.Launch.Number("budget.usd_list_cap"); ok && cap > 0 && a.CostUSDList > cap {
		out = append(out, fmt.Sprintf("list cost US$ %s is over the cap of US$ %s",
			jsconv.Number(a.CostUSDList), jsconv.Number(cap)))
	}
	return out
}

func byID(id string, prices Prices, d Deps) (string, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return "", err
	}
	l, ok := registry.Find(launches, id)
	if !ok {
		return "", fail.Rejectedf("unknown launch %s", id)
	}
	transcript, _ := l.Launch.String("transcript_path")
	if _, err := os.Stat(transcript); transcript == "" || err != nil {
		return "", fail.Rejectedf("launch %s has no transcript on disk yet", id)
	}
	a, err := Session(transcript, prices)
	if err != nil {
		return "", err
	}
	cat, err := d.Catalog()
	if err != nil {
		return "", err
	}
	var checks []catalog.SuccessCheck
	if build, _ := l.Launch.String("build"); build != "" {
		if b, ok := cat.Find(build); ok {
			checks = b.Success
		}
	}
	overBudget := budgetBreaks(l, a)
	verdict, reason := verdictOf(a.SecretPathReads, len(overBudget) > 0, checks)
	ts := jsonx.Time(d.Now())

	var budgetPct *float64
	if tokens, ok := l.Launch.Number("budget.tokens"); ok && tokens > 0 {
		pct := jsRound(a.MaxMainContextTokens/tokens*1000) / 10
		budgetPct = &pct
	}

	breaks := []registry.RuleBreakCount{}
	if a.SecretPathReads > 0 {
		evidence := fmt.Sprintf("%d tool call(s) touched a secret path", a.SecretPathReads)
		breaks = append(breaks, registry.RuleBreakCount{Kind: "secret_path_read", Count: int64(a.SecretPathReads)})
		if !hasRuleBreak(l, "secret_path_read", evidence) {
			rb := &registry.RuleBreakEvent{ID: &id, TS: ts, Kind: "secret_path_read", Evidence: evidence}
			if err := d.Store.Append(rb); err != nil {
				return "", err
			}
		}
	}
	if len(overBudget) > 0 {
		breaks = append(breaks, registry.RuleBreakCount{Kind: "budget_over", Count: int64(len(overBudget))})
		for _, evidence := range overBudget {
			if hasRuleBreak(l, "budget_over", evidence) {
				continue
			}
			rb := &registry.RuleBreakEvent{ID: &id, TS: ts, Kind: "budget_over", Evidence: evidence}
			if err := d.Store.Append(rb); err != nil {
				return "", err
			}
		}
	}
	err = d.Store.Append(&registry.AnalyzedEvent{
		ID:          id,
		TS:          ts,
		Analyzer:    Analyzer,
		Turns:       int64(a.MainRequests),
		Requests:    int64(a.Requests),
		PeakContext: int64(a.MaxMainContextTokens),
		BudgetPct:   budgetPct,
		Tokens: registry.TokenCounts{
			Input:      int64(a.Tokens.Input),
			CacheRead:  int64(a.Tokens.CacheRead),
			CacheWrite: int64(a.Tokens.CacheWrite),
			Output:     int64(a.Tokens.Output),
		},
		CostUSDList:   a.CostUSDList,
		PriceTable:    prices.TableLabel(),
		DurationMin:   a.DurationMin,
		SubagentFiles: int64(a.SubagentFiles),
		ByModel:       a.ByModel,
		ToolsUsed:     a.ToolUseCounts,
		RuleBreaks:    breaks,
		Verdict:       verdict,
		VerdictReason: reason,
	})
	if err != nil {
		return "", err
	}

	line := fmt.Sprintf("jitso: analyzed %s: %d turns, peak %s tokens, US$ %s list, verdict %s",
		id, a.MainRequests, jsconv.Number(a.MaxMainContextTokens), jsconv.Number(a.CostUSDList), verdict)
	var notes []string
	if a.SecretPathReads > 0 {
		notes = append(notes, fmt.Sprintf("%d secret-path read(s)", a.SecretPathReads))
	}
	if len(overBudget) > 0 {
		notes = append(notes, "over budget")
	}
	if len(notes) > 0 {
		line += " (" + strings.Join(notes, ", ") + ")"
	}
	return line, nil
}

// hasRuleBreak is true when the launch already has this rule_break line, so a
// second analysis does not repeat it.
func hasRuleBreak(l *registry.Launch, kind, evidence string) bool {
	for _, e := range l.RuleBreaks {
		k, _ := e.String("kind")
		ev, _ := e.String("evidence")
		if k == kind && ev == evidence {
			return true
		}
	}
	return false
}
