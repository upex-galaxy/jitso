package settingsjson

import "encoding/json"

// writtenLines are the lines, trimmed of their spaces and a trailing comma,
// that Register writes besides the handler command and the braces.
var writtenLines = map[string]bool{
	`"hooks": {`:                       true,
	`"hooks": {}`:                      true,
	`"hooks": [`:                       true,
	`"` + EventPreToolUse + `": [`:     true,
	`"` + EventSessionEnd + `": [`:     true,
	`"matcher": "` + MatcherBash + `"`: true,
	`"matcher": "` + MatcherMCP + `"`:  true,
	`"type": "command"`:                true,
}

// WrittenLine returns the test for the lines Register(_, command) writes: a
// key or value of JITSO's own groups, or a handler line whose command is a
// JITSO command or command itself. Whatever else a line says belongs to the
// user, who may keep secrets there. The argument of the test is a line of the
// settings file trimmed of its spaces and a trailing comma.
func WrittenLine(command string) func(line string) bool {
	return func(line string) bool {
		if writtenLines[line] {
			return true
		}
		const key = `"command": `
		if len(line) <= len(key) || line[:len(key)] != key {
			return false
		}
		var cmd string
		if json.Unmarshal([]byte(line[len(key):]), &cmd) != nil {
			return false
		}
		return IsJitsoCommand(cmd) || (command != "" && cmd == command)
	}
}
