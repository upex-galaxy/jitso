// Package hook is the decision logic and the I/O shell of the two Claude Code
// hooks that ship inside the jitso binary: `jitso hook guard` (the launch
// guard and the SessionEnd registry writer) and `jitso hook dispatch-guard`
// (the guard of the interactive dispatcher).
//
// Both read the hook JSON on stdin and answer on stdout as SPEC section 6.2
// says: nothing and exit 0 to stay out of the way, or one JSON object with a
// permissionDecision. The decisions are pure functions of the parsed input;
// stdin, stdout, the filesystem and the clock are injected by the caller.
package hook

import "github.com/upex-galaxy/jitso/internal/jsonx"

// The decisions a hook prints.
const (
	Allow = "allow"
	Deny  = "deny"
)

// Answer is the line a hook prints for decision (Allow or Deny) with reason:
// one JSON object and a newline, the shape Claude Code reads.
func Answer(decision, reason string) []byte {
	out, err := jsonx.Marshal(jsonx.Object{{Key: "hookSpecificOutput", Value: jsonx.Object{
		{Key: "hookEventName", Value: "PreToolUse"},
		{Key: "permissionDecision", Value: decision},
		{Key: "permissionDecisionReason", Value: reason},
	}}})
	if err != nil {
		// A string-only object always encodes; keep the hook closed anyway.
		return []byte(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"JITSO hook: cannot encode the answer"}}` + "\n")
	}
	return append(out, '\n')
}

// field is the value of a key of a decoded JSON object, nil when it is not an
// object or has no such key (JavaScript's `value?.key`).
func field(v any, key string) any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m[key]
}

// text is v when it is a string (JavaScript `typeof v === 'string'`).
func text(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}
