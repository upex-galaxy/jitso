// Package profile turns a launch request and a loaded catalog into a
// validated profile or a rejection (exit 2, "JITSO REJECTED: <reason>"). It
// is pure: the catalog is already loaded and validated.
//
// The dispatcher model asks; this code decides. A request picks a build and
// gives the fields that build requires; every field value must match its
// pattern, so nothing a model wrote carries shell syntax further.
package profile

import (
	"slices"

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

// Resolve validates req against cat, in the oracle's order: the build exists,
// is approved, is not subagent tier (a subagent-tier build is never launched
// alone, defect N-5), its model and effort are allowlisted, every MCP server
// is known, the route scope is valid, every field is required by the build
// and matches its pattern, and every required field is given.
func Resolve(req Request, cat *catalog.Catalog) (Profile, error) {
	b, ok := cat.Find(req.Build)
	if !ok {
		return Profile{}, fail.Rejectedf(`unknown build "%s"`, req.Build)
	}
	if b.Status != "approved" {
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
	if len(scope) == 0 {
		return Profile{}, fail.Rejected("empty route scope (use none)")
	}
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
		pattern, ok := catalog.FieldPattern(name)
		if !ok || !pattern.MatchString(req.Fields.Get(name)) {
			return Profile{}, fail.Rejectedf("field %s has an invalid value", name)
		}
	}
	for _, name := range b.Requires {
		if !req.Fields.Has(name) {
			return Profile{}, fail.Rejectedf("missing %s", name)
		}
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
