package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
)

// ConfigDirEnv is the variable that moves Claude Code's user configuration
// folder, <home>/.claude when unset.
const ConfigDirEnv = "CLAUDE_CONFIG_DIR"

// A launched session runs with --setting-sources project, so it never reads the
// owner's user settings: the launch copies them into its own settings file, which
// --settings takes with the precedence of a flag. These are the top-level keys
// the copy leaves out.
var (
	// contextKeys pick what a session loads: plugins, marketplaces, skill
	// overrides and the project MCP servers. A build says that.
	contextKeys = []string{
		"enabledPlugins", "extraKnownMarketplaces", "pluginConfigs", "skillOverrides",
		"enableAllProjectMcpServers", "enabledMcpjsonServers", "disabledMcpjsonServers",
		"allowedMcpServers", "deniedMcpServers",
	}
	// workerKeys are the owner's choices for a person at the keyboard.
	workerKeys = []string{"language", "askUserQuestionTimeout", "remoteControlAtStartup"}
	// decidedKeys are decided by the build; permissions.defaultMode is too,
	// and is cut from inside permissions.
	decidedKeys = []string{"model", "effortLevel"}
	// ownKeys are set by JITSO itself, on top of what is copied.
	ownKeys = []string{"skipDangerousModePermissionPrompt", "promptSuggestionEnabled"}
	// mergedKeys are merged by Claude Code across scopes, so a project that
	// defines them does not hide the owner's (env is merged by variable, and
	// the project's variables are dropped from the copy).
	mergedKeys = []string{"hooks", "permissions", "env"}
)

var _ harness.SettingsSource = Adapter{}

// configDir is the folder Claude Code keeps the user configuration in:
// CLAUDE_CONFIG_DIR when that is set and not empty, else <home>/.claude, and ""
// without a home (a relative path would be the project's own file).
func configDir(home string, lookup func(string) (string, bool)) string {
	if lookup != nil {
		if dir, _ := lookup(ConfigDirEnv); dir != "" {
			return dir
		}
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// OwnerSettings reads the owner's user settings (settings.json of
// CLAUDE_CONFIG_DIR when that is set and not empty, else of <home>/.claude)
// and the keys the checkout's own .claude/settings.json defines, and returns
// what travels (Filter). A missing file, a path that is not a readable regular
// file and a top level that is not an object travel nothing; a file that is
// not JSON travels nothing and comes with a warning that names the path and
// quotes nothing of it. The checkout's .claude/settings.local.json is never
// opened.
func (Adapter) OwnerSettings(in harness.OwnerSettingsInput) harness.OwnerSettings {
	dir := configDir(in.Home, in.LookupEnv)
	if dir == "" {
		return harness.OwnerSettings{}
	}
	path := filepath.Join(dir, "settings.json")
	user, valid := readObject(path)
	if !valid {
		return harness.OwnerSettings{Warning: "jitso: warning: " + path + " is not valid JSON, so the session gets none of the settings in it"}
	}
	var project map[string]any
	if in.Checkout != "" {
		project, _ = readObject(filepath.Join(in.Checkout, ".claude", "settings.json"))
	}
	doc, copied, cut := Filter(user, project, in.Exclude, in.Include)
	return harness.OwnerSettings{Doc: doc, Copied: copied, Cut: cut}
}

// readObject is the top-level object of a JSON file, numbers kept as written.
// nil for no file, a path that is not a regular file and a top level that is
// not an object; valid is false only for a file that is not JSON.
func readObject(path string) (obj map[string]any, valid bool) {
	// Reading a pipe or a device could block the launch.
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return nil, true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, true
	}
	if !json.Valid(data) {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if dec.Decode(&obj) != nil {
		return nil, true
	}
	return obj, true
}

// Filter is what travels from the owner's top-level settings user, given the
// top-level settings the project defines (project, nil for none) and the
// owner's JITSO config (exclude cuts more keys, include lets cut keys travel).
//
// Every key travels, unknown ones included, except: the context keys and the
// per-worker keys (unless included), the keys the build decides (model,
// effortLevel, and permissions.defaultMode inside permissions), the keys JITSO
// sets itself, the extra keys of exclude, and any key the project defines,
// except hooks and permissions, which Claude Code merges across scopes. From
// env only the variables the project env defines are dropped. Guard handlers
// are removed from the owner's hooks, so that the guard the launch carries does
// not run twice; the groups and events that removal empties go. include never
// overrides the build, the project or JITSO; exclude beats include.
//
// copied and cut are sorted key names: copied are the keys of user that
// travel, cut the keys of user that do not, whatever the reason.
func Filter(user, project map[string]any, exclude, include []string) (doc map[string]any, copied, cut []string) {
	doc = map[string]any{}
	for key, value := range user {
		switch {
		case slices.Contains(decidedKeys, key), slices.Contains(ownKeys, key):
		case slices.Contains(exclude, key):
		case !slices.Contains(mergedKeys, key) && hasKey(project, key):
			// the project wins the key; env and the merged keys are cut inside.
		case (slices.Contains(contextKeys, key) || slices.Contains(workerKeys, key)) && !slices.Contains(include, key):
		default:
			if kept, ok := keep(key, value, project); ok {
				doc[key] = kept
			}
		}
	}
	for key := range user {
		if _, ok := doc[key]; ok {
			copied = append(copied, key)
		} else {
			cut = append(cut, key)
		}
	}
	sort.Strings(copied)
	sort.Strings(cut)
	return doc, copied, cut
}

func hasKey(obj map[string]any, key string) bool {
	_, ok := obj[key]
	return ok
}

// keep is the value of a key that travels, after the cuts inside it: false
// when nothing of it is left.
func keep(key string, value any, project map[string]any) (any, bool) {
	object, isObject := value.(map[string]any)
	if !isObject {
		return value, true
	}
	switch key {
	case "permissions":
		out := map[string]any{}
		for k, v := range object {
			if k != "defaultMode" {
				out[k] = v
			}
		}
		return out, len(out) > 0 || len(object) == 0
	case "env":
		projectEnv, _ := project["env"].(map[string]any)
		out := map[string]any{}
		for k, v := range object {
			if !hasKey(projectEnv, k) {
				out[k] = v
			}
		}
		return out, len(out) > 0 || len(object) == 0
	case "hooks":
		out := withoutGuard(object)
		return out, len(out) > 0 || len(object) == 0
	}
	return value, true
}

// withoutGuard is hooks without the handlers that run the JITSO guard. A group
// or an event this empties is dropped; one the owner left empty stays.
func withoutGuard(hooks map[string]any) map[string]any {
	out := map[string]any{}
	for event, v := range hooks {
		groups, isList := v.([]any)
		if !isList {
			out[event] = v
			continue
		}
		kept := make([]any, 0, len(groups))
		for _, g := range groups {
			group, isGroup := g.(map[string]any)
			handlers, hasHandlers := group["hooks"].([]any)
			if !isGroup || !hasHandlers {
				kept = append(kept, g)
				continue
			}
			left := make([]any, 0, len(handlers))
			for _, h := range handlers {
				if !isGuard(h) {
					left = append(left, h)
				}
			}
			if len(left) == len(handlers) {
				kept = append(kept, g)
				continue
			}
			if len(left) == 0 {
				continue
			}
			copyGroup := map[string]any{}
			for k, gv := range group {
				copyGroup[k] = gv
			}
			copyGroup["hooks"] = left
			kept = append(kept, copyGroup)
		}
		if len(kept) == 0 && len(groups) > 0 {
			continue
		}
		out[event] = kept
	}
	return out
}

// isGuard: a command handler whose command runs the JITSO guard.
func isGuard(handler any) bool {
	h, ok := handler.(map[string]any)
	if !ok {
		return false
	}
	kind, _ := h["type"].(string)
	command, _ := h["command"].(string)
	return kind == "command" && settingsjson.IsJitsoCommand(command)
}
