package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/upex-galaxy/jitso/internal/harness"
)

// InboundKey is the user setting that says what Claude Code does with a message
// from another session. A launched session runs with --setting-sources project,
// so it never reads the owner's user settings: the launch carries this one key
// in --settings, which takes precedence over them.
const InboundKey = "crossSessionInbound"

// ConfigDirEnv is the variable that moves Claude Code's user configuration
// folder, <home>/.claude when unset.
const ConfigDirEnv = "CLAUDE_CONFIG_DIR"

// inboundValues are the values Claude Code knows for InboundKey.
var inboundValues = []string{"accept", "hold", "refuse"}

var _ harness.InboundSource = Adapter{}

// OwnerInbound reads InboundKey from the owner's user settings: settings.json
// of CLAUDE_CONFIG_DIR when that is set and not empty, else of <home>/.claude.
// It returns "" for no file, a path that is not a readable regular file, a file
// without the key and any value but accept, hold or refuse, and "" with a
// warning for a file that is not JSON. No other key or value leaves this
// function, and the warning quotes nothing of the file.
func (Adapter) OwnerInbound(home string, lookupEnv func(string) (string, bool)) (value, warning string) {
	dir, _ := lookupEnv(ConfigDirEnv)
	if dir == "" {
		// Without a home the relative path would be the project's own file.
		if home == "" {
			return "", ""
		}
		dir = filepath.Join(home, ".claude")
	}
	path := filepath.Join(dir, "settings.json")
	// Reading a pipe or a device could block the launch.
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	if !json.Valid(data) {
		return "", "jitso: warning: " + path + " is not valid JSON, so the session gets no " + InboundKey
	}
	// A map matches the key by its exact spelling; a top level that is not an
	// object holds no key.
	var keys map[string]json.RawMessage
	if json.Unmarshal(data, &keys) != nil {
		return "", ""
	}
	if json.Unmarshal(keys[InboundKey], &value) != nil || !slices.Contains(inboundValues, value) {
		return "", ""
	}
	return value, ""
}
