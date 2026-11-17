package settingsjson

import (
	"regexp"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// The events and matchers of the groups JITSO owns.
const (
	EventPreToolUse = "PreToolUse"
	EventSessionEnd = "SessionEnd"
	// MatcherBash is the exact tool name of the Bash group.
	MatcherBash = "Bash"
	// MatcherMCP is the regex of the orca MCP tools the guard judges.
	MatcherMCP = "mcp__.*orca.*__.*"
)

// Command is the handler command jitso writes: the bare word.
const Command = "jitso hook guard"

// jitsoCommand recognizes a command that runs `jitso hook guard`: the bare
// word, or the path of the binary (a quoted or an absolute one) ending in
// jitso, which is what an earlier jitso or --absolute wrote.
var jitsoCommand = regexp.MustCompile(`^(?:jitso|'(?:[^']|'\\'')*/jitso[^'/]*'|"[^"]*/jitso[^"/]*"|/[^ '"]*/jitso[^ /'"]*) hook guard$`)

// IsJitsoCommand reports whether command is a JITSO hook command.
func IsJitsoCommand(command string) bool { return jitsoCommand.MatchString(command) }

// isHandler: a handler is a JITSO handler when it is a command handler whose
// command is a JITSO command (or the exact command being written, so a binary
// with another name is still found again).
func isHandler(h any, extra string) bool {
	obj, ok := h.(jsonx.Object)
	if !ok {
		return false
	}
	t, _ := lookup(obj, "type")
	if s, _ := t.(string); s != "command" {
		return false
	}
	c, _ := lookup(obj, "command")
	cmd, ok := c.(string)
	return ok && (IsJitsoCommand(cmd) || (extra != "" && cmd == extra))
}

// handlers is the handler list of a group (nil when it has none).
func handlers(group jsonx.Object) []any {
	v, _ := lookup(group, "hooks")
	list, _ := v.([]any)
	return list
}

// isGroup: a JITSO group is a group with at least one handler and only JITSO
// handlers.
func isGroup(group jsonx.Object, extra string) bool {
	list := handlers(group)
	if len(list) == 0 {
		return false
	}
	for _, h := range list {
		if !isHandler(h, extra) {
			return false
		}
	}
	return true
}

// group builds a canonical JITSO group; an empty matcher leaves the key out.
func group(matcher, command string) jsonx.Object {
	var g jsonx.Object
	if matcher != "" {
		g = append(g, jsonx.Field{Key: "matcher", Value: matcher})
	}
	return append(g, jsonx.Field{Key: "hooks", Value: []any{jsonx.Object{
		{Key: "type", Value: "command"},
		{Key: "command", Value: command},
	}}})
}

// wanted is the groups the desired state holds, per event, in the order they
// are appended.
func wanted(command string) []struct {
	event  string
	groups []jsonx.Object
} {
	return []struct {
		event  string
		groups []jsonx.Object
	}{
		{EventPreToolUse, []jsonx.Object{group(MatcherBash, command), group(MatcherMCP, command)}},
		{EventSessionEnd, []jsonx.Object{group("", command)}},
	}
}

// Fragment is the document jitso writes into a project with no settings file:
// the three groups, 2-space indent, trailing newline. It is also what a person
// adds by hand when the layout of a file is refused.
func Fragment(command string) []byte {
	doc, err := Register(nil, command)
	if err != nil { // the tree is built here, it always encodes
		panic("settingsjson: " + err.Error())
	}
	return doc
}

// strip removes every JITSO handler under hooks. A group whose handlers were
// all removed goes, and so does an event whose array the removal emptied,
// except the events in keep, which stay (empty) so that a group written again
// keeps its place. A group, an event or a file the user left empty is not
// touched. It returns the new hooks object and the number of handlers removed.
func strip(hooks jsonx.Object, extra string, keep map[string]bool) (jsonx.Object, int) {
	removed := 0
	out := make(jsonx.Object, 0, len(hooks))
	for _, ev := range hooks {
		groups := ev.Value.([]any)
		kept := make([]any, 0, len(groups))
		hit := 0
		for _, g := range groups {
			grp := g.(jsonx.Object)
			list := handlers(grp)
			var rest []any
			n := 0
			for _, h := range list {
				if isHandler(h, extra) {
					n++
					continue
				}
				rest = append(rest, h)
			}
			if n == 0 {
				kept = append(kept, g)
				continue
			}
			hit += n
			if len(rest) == 0 {
				continue
			}
			kept = append(kept, with(grp, "hooks", rest))
		}
		removed += hit
		if hit > 0 && len(kept) == 0 && !keep[ev.Key] {
			continue
		}
		out = append(out, jsonx.Field{Key: ev.Key, Value: kept})
	}
	return out, removed
}

// hooksOf is the hooks object of a root (nil when it has none).
func hooksOf(root jsonx.Object) jsonx.Object {
	v, _ := lookup(root, "hooks")
	obj, _ := v.(jsonx.Object)
	return obj
}

// desired reports whether the desired state already holds: exactly one JITSO
// group each with the Bash and the orca matcher under PreToolUse, exactly one
// without matcher under SessionEnd, each exactly the canonical
// group for command, and no other JITSO handler anywhere under hooks.
func desired(hooks jsonx.Object, command string) bool {
	total := 0
	for _, ev := range hooks {
		for _, g := range ev.Value.([]any) {
			for _, h := range handlers(g.(jsonx.Object)) {
				if isHandler(h, command) {
					total++
				}
			}
		}
	}
	if total != 3 {
		return false
	}
	for _, w := range wanted(command) {
		v, _ := lookup(hooks, w.event)
		have := map[string]int{}
		for _, g := range asList(v) {
			b, _ := jsonx.Marshal(g)
			have[string(b)]++
		}
		for _, g := range w.groups {
			b, _ := jsonx.Marshal(g)
			if have[string(b)] != 1 {
				return false
			}
		}
	}
	return true
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

// Register returns the settings file with the guard registered for command:
// the three JITSO groups appended after the user's own, every other JITSO
// handler (an older absolute path, a duplicate) taken out first. A nil doc is
// an absent file and gives the file jitso creates.
//
// The order of the checks matters. A file that is not JSON or has the wrong
// shape is a *Problem first. When the desired state already holds, doc comes
// back as it is (the same bytes, whatever its layout). Only a file that has to
// change must also re-encode to its own bytes, else it is refused with a
// KindLayout problem.
func Register(doc []byte, command string) ([]byte, error) {
	var d *document
	if doc == nil {
		d = &document{root: jsonx.Object{}, layout: Layout{Indent: DefaultIndent, Newline: true}}
	} else {
		var p *Problem
		if d, p = parse(doc); p != nil {
			return nil, p
		}
	}
	hooks := hooksOf(d.root)
	if doc != nil && desired(hooks, command) {
		return doc, nil
	}
	if doc != nil {
		if p := d.checkLayout(); p != nil {
			return nil, p
		}
	}
	if hooks == nil {
		hooks = jsonx.Object{}
	}
	keep := map[string]bool{EventPreToolUse: true, EventSessionEnd: true}
	hooks, _ = strip(hooks, command, keep)
	for _, w := range wanted(command) {
		var list []any
		if v, ok := lookup(hooks, w.event); ok {
			list = append(list, asList(v)...)
		}
		for _, g := range w.groups {
			list = append(list, g)
		}
		hooks = with(hooks, w.event, list)
	}
	return d.render(with(d.root, "hooks", hooks))
}

// Unregister returns the settings file without any JITSO handler: groups that
// held only JITSO handlers go, and so do the events and the hooks object the
// removal emptied. When nothing is left in the document it returns nil, which
// means the file itself should be deleted. A file with no JITSO handler (and
// a nil doc) comes back as it is, whatever its layout; a file that has to
// change is refused when it does not re-encode to its own bytes.
func Unregister(doc []byte) ([]byte, error) {
	if doc == nil {
		return nil, nil
	}
	d, p := parse(doc)
	if p != nil {
		return nil, p
	}
	hooks := hooksOf(d.root)
	if hooks == nil {
		return doc, nil
	}
	hooks, removed := strip(hooks, "", nil)
	if removed == 0 {
		return doc, nil
	}
	if p := d.checkLayout(); p != nil {
		return nil, p
	}
	root := d.root
	if len(hooks) == 0 {
		root = without(root, "hooks")
	} else {
		root = with(root, "hooks", hooks)
	}
	if len(root) == 0 {
		return nil, nil
	}
	return d.render(root)
}

// HookState is what a settings file says about the guard, for `jitso doctor`.
type HookState struct {
	// Err is why the file could not be read as settings (a *Problem); the
	// other fields are then empty.
	Err error
	// Bash, MCP and SessionEnd: a JITSO group is registered on that slot (the
	// Bash and the orca MCP matcher of PreToolUse, SessionEnd with no matcher).
	Bash, MCP, SessionEnd bool
	// Commands are the distinct JITSO handler commands found, in file order.
	Commands []string
	// DisableAllHooks is true when the file sets disableAllHooks to true.
	DisableAllHooks bool
}

// Registered reports whether the guard is on all three slots.
func (s HookState) Registered() bool {
	return s.Err == nil && s.Bash && s.MCP && s.SessionEnd
}

// None reports whether the file holds no JITSO handler at all.
func (s HookState) None() bool { return len(s.Commands) == 0 }

// State reads what a file says about the guard. A nil doc is an absent file.
func State(doc []byte) HookState {
	var s HookState
	if doc == nil {
		return s
	}
	d, p := parse(doc)
	if p != nil {
		s.Err = p
		return s
	}
	if v, ok := lookup(d.root, "disableAllHooks"); ok {
		s.DisableAllHooks = v == true
	}
	seen := map[string]bool{}
	for _, ev := range hooksOf(d.root) {
		for _, g := range ev.Value.([]any) {
			grp := g.(jsonx.Object)
			for _, h := range handlers(grp) {
				if !isHandler(h, "") {
					continue
				}
				c, _ := lookup(h.(jsonx.Object), "command")
				if cmd := c.(string); !seen[cmd] {
					seen[cmd] = true
					s.Commands = append(s.Commands, cmd)
				}
			}
			if !isGroup(grp, "") {
				continue
			}
			m, hasMatcher := lookup(grp, "matcher")
			switch {
			case ev.Key == EventPreToolUse && m == MatcherBash:
				s.Bash = true
			case ev.Key == EventPreToolUse && m == MatcherMCP:
				s.MCP = true
			case ev.Key == EventSessionEnd && !hasMatcher:
				s.SessionEnd = true
			}
		}
	}
	return s
}
