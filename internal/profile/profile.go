// Package profile turns a launch request and a loaded catalog into a
// validated profile or a rejection (exit 2, "JITSO REJECTED: <reason>"). It
// is pure: the catalog is already loaded and validated.
//
// The launching model asks; this code decides. A request picks a build and
// gives the fields that build requires; every field value must match the
// pattern the project's jitso/jitso.yaml gives its field, or the generic check
// (not empty, one line) when it gives none.
package profile

import (
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// Request is what a launch asks for.
type Request struct {
	// Build is the build id.
	Build string
	// Fields are the --field name=value pairs.
	Fields Fields
}

// Profile is a resolved, validated request.
type Profile struct {
	// Build is the build the request named.
	Build *catalog.Build
	// Model, Effort, MCP and RouteScope are the values the session gets.
	Model      string
	Effort     string
	MCP        []string
	RouteScope []string
	// Subagents are the subagent-tier builds the build lists, in its order.
	Subagents []*catalog.Build
	Fields    Fields
}

// Resolve validates req against cat, in this order: the build exists
// (the nearest layer that has the id; invalid for this project is a rejection
// that names the file and its problems, defect N-12), is approved (a draft of the draft layer launches too, and only that layer:
// defect N-15), is not subagent tier (a subagent-tier build is never launched
// alone, defect N-5), its model and effort are allowlisted, every MCP server
// is known, the route scope is valid, every field is required by the build
// and matches its pattern, every required field is given, and every subagent
// the build lists is approved (defect N-11): a subagent's prompt reaches
// `claude --agents`, so only reviewed text may. A draft worker may also list
// draft subagents; a saved worker may not.
func Resolve(req Request, cat *catalog.Catalog) (Profile, error) {
	b, ok := cat.Find(req.Build)
	if !ok {
		// The nearest file with this id exists but is invalid for this
		// project: the id is unusable here, and a farther file is not used.
		if s, invalid := cat.Invalid(req.Build); invalid {
			return Profile{}, fail.Rejectedf(`build "%s" from %s %s is invalid here: %s`, s.ID, s.Layer, s.Display, strings.Join(s.Problems, "; "))
		}
		return Profile{}, fail.Rejectedf(`unknown build "%s"`, req.Build)
	}
	if !launchableStatus(b) {
		return Profile{}, fail.Rejectedf(`build "%s" is %s, not approved`, b.ID, b.Status)
	}
	if b.Tier == catalog.TierSubagent {
		return Profile{}, fail.Rejectedf(`build "%s" has tier subagent: it is not launched alone, a worker lists it in its subagents`, b.ID)
	}
	cfg := cat.Config
	if !slices.Contains(cfg.Models, b.Model) {
		return Profile{}, fail.Rejectedf(`model "%s" is not an allowlisted full id`, b.Model)
	}
	if !slices.Contains(cfg.Efforts, b.Effort) {
		return Profile{}, fail.Rejectedf(`effort "%s" is not allowed`, b.Effort)
	}
	for _, name := range b.MCP {
		if !slices.Contains(cfg.MCP, name) {
			return Profile{}, fail.Rejectedf(`unknown mcp "%s"`, name)
		}
	}
	scope := b.RouteScope
	for _, id := range scope {
		if id != "none" && !slices.Contains(cat.RouteIDs, id) {
			return Profile{}, fail.Rejectedf(`unknown route-scope id "%s"`, id)
		}
	}
	if slices.Contains(scope, "none") && len(scope) > 1 {
		return Profile{}, fail.Rejected(`"none" cannot be combined with route-scope ids`)
	}
	for _, name := range req.Fields.Keys() {
		if !slices.Contains(b.Requires, name) {
			return Profile{}, fail.Rejectedf(`unknown field "%s" for build "%s"`, name, b.ID)
		}
		value := req.Fields.Get(name)
		pattern, ok := cfg.FieldPattern(name)
		if (ok && !pattern.MatchString(value)) || (!ok && !catalog.ValidFieldValue(value)) {
			return Profile{}, fail.Rejectedf("field %s has an invalid value", name)
		}
	}
	for _, name := range b.Requires {
		if !req.Fields.Has(name) {
			return Profile{}, fail.Rejectedf("missing %s", name)
		}
	}
	if s := unapprovedSubagents(b, cat); len(s) > 0 {
		return Profile{}, fail.Rejectedf(`build "%s" lists subagent "%s", which is %s, not approved`, b.ID, s[0].ID, s[0].Status)
	}
	return Profile{
		Build:      b,
		Model:      b.Model,
		Effort:     b.Effort,
		MCP:        b.MCP,
		RouteScope: scope,
		Subagents:  cat.SubagentsOf(b),
		Fields:     req.Fields,
	}, nil
}

// launchableStatus is true for the status a build may launch with: approved in
// any layer, and draft in the draft layer, the one layer whose files are drafts.
func launchableStatus(b *catalog.Build) bool {
	return b.Status == "approved" || (b.LayerWord() == catalog.LayerDraft && b.Status == catalog.StatusDraft)
}

// LaunchProblems lists what keeps the build b from launching and that only a
// launch can see, as plain problem lines (the texts `jitso build validate`
// prints): each subagent it lists that is not approved.
func LaunchProblems(b *catalog.Build, cat *catalog.Catalog) []string {
	var problems []string
	for _, s := range unapprovedSubagents(b, cat) {
		problems = append(problems, `subagent "`+s.ID+`" is `+s.Status+", not approved")
	}
	return problems
}

// unapprovedSubagents are the subagents b lists whose status is not approved,
// in b's order. A draft build may list draft subagents: its own prompt is not
// reviewed either, and it launches only while it stays a draft.
func unapprovedSubagents(b *catalog.Build, cat *catalog.Catalog) []*catalog.Build {
	var out []*catalog.Build
	for _, s := range cat.SubagentsOf(b) {
		if s.Status == "approved" || (b.LayerWord() == catalog.LayerDraft && s.Status == catalog.StatusDraft) {
			continue
		}
		out = append(out, s)
	}
	return out
}
