package expect

import (
	"fmt"
	"regexp"
	"strings"

	"jitso-contract-runner/internal/jsonx"
)

// routeScopeLine is the router's own regex (personality-reinject.mjs `scopeOf`), flags gim.
var routeScopeLine = regexp.MustCompile(`(?im)(?:^|[.!?])[ \t>*-]*ROUTE-SCOPE:([^\n]*)`)

var trailingPunct = regexp.MustCompile(`[.;]+$`)

func init() {
	registerText("route_scope", func(v any, what string) (TextMatcher, error) {
		ids, err := jsonx.Strings(v, what)
		if err != nil {
			return nil, err
		}
		return routeScopeMatcher{want: ids}, nil
	})
}

// RouteScope reads text like the router: every ROUTE-SCOPE: capture, joined, split on commas,
// each part trimmed then stripped of trailing `.`/`;`, empty parts and `none` dropped.
// found is false when no ROUTE-SCOPE: line exists at all.
func RouteScope(text string) (ids []string, found bool) {
	matches := routeScopeLine.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, false
	}
	captures := make([]string, len(matches))
	for i, m := range matches {
		captures[i] = m[1]
	}
	ids = []string{}
	for _, part := range strings.Split(strings.Join(captures, ","), ",") {
		item := trailingPunct.ReplaceAllString(strings.TrimSpace(part), "")
		if item == "" || strings.EqualFold(item, "none") {
			continue
		}
		ids = append(ids, item)
	}
	return ids, true
}

type routeScopeMatcher struct{ want []string }

func (m routeScopeMatcher) Name() string { return "route_scope" }

func (m routeScopeMatcher) Match(text string) *Mismatch {
	expected := fmt.Sprintf("ROUTE-SCOPE: ids %q", m.want)
	ids, found := RouteScope(text)
	if !found {
		return &Mismatch{Expected: expected, Actual: "no line matches the router regex (is the colon missing?):\n" + text}
	}
	if len(ids) != len(m.want) {
		return &Mismatch{Expected: expected, Actual: fmt.Sprintf("ids %q", ids)}
	}
	for i := range ids {
		if ids[i] != m.want[i] {
			return &Mismatch{Expected: expected, Actual: fmt.Sprintf("ids %q", ids)}
		}
	}
	return nil
}
