package claude

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
)

var _ harness.UserLayerSource = Adapter{}

// UserLayer is the overrides of a launch that loads claude.ai connectors.
// Connectors load only when the user settings layer is a setting source, so
// the session reads the owner's user settings as they are (preferences, env,
// hooks), and settings.json cuts what a build decides: it never copies them,
// because copying would run the owner's hooks twice. The overrides, each only
// when there is something to put in it:
//
//   - allowedMcpServers: the servers of the launch's mcp file by name and the
//     connectors by URL, so no other server of the user layer loads;
//   - enabledPlugins: every plugin the owner's user settings name, off;
//   - claudeMdExcludes: <config dir>/CLAUDE.md and the files it imports with
//     @<path> (one level only: an imported file's own imports are not read);
//   - skillOverrides: every skill folder of <config dir>/skills and
//     ~/.agents/skills off, except the ones the launch carries and the ones the
//     checkout owns;
//   - permissions.deny: Agent(<name>) for each agent of <config dir>/agents;
//   - remoteControlAtStartup false.
//
// language and askUserQuestionTimeout pass: the settings reference documents no
// neutral value for them. The skills bundled with Claude Code and claude.ai,
// and the MCP servers of built-in plugins, cannot be cut this way.
func (Adapter) UserLayer(in harness.UserLayerInput) harness.UserLayer {
	out := harness.UserLayer{Doc: map[string]any{}}
	for _, c := range in.Connectors {
		out.Cloud = append(out.Cloud, c.Name)
	}
	allowed := []any{}
	for _, name := range in.Servers {
		allowed = append(allowed, map[string]any{"serverName": name})
	}
	for _, c := range in.Connectors {
		allowed = append(allowed, map[string]any{"serverUrl": c.URL})
	}
	out.Doc["allowedMcpServers"] = allowed
	out.Doc["remoteControlAtStartup"] = false

	dir := configDir(in.Home, in.LookupEnv)
	if dir != "" {
		userSettings := filepath.Join(dir, "settings.json")
		user, valid := readObject(userSettings)
		if !valid {
			out.Warning = "jitso: warning: " + userSettings + " is not valid JSON, so no plugin of it is switched off for the session"
		}
		if plugins, _ := user["enabledPlugins"].(map[string]any); len(plugins) > 0 {
			off := map[string]any{}
			for key := range plugins {
				off[key] = false
			}
			out.Doc["enabledPlugins"] = off
		}
		if data, err := os.ReadFile(userSettings); err == nil {
			out.GuardRegistered = settingsjson.State(data).Registered()
		}
		out.Doc["claudeMdExcludes"] = toAny(claudeMDExcludes(dir, in.Home))
		keep := map[string]bool{}
		for _, name := range in.KeepSkills {
			keep[name] = true
		}
		for _, name := range skillFolders(in.Checkout, ".claude", "skills") {
			keep[name] = true
		}
		for _, name := range skillFolders(in.Checkout, ".agents", "skills") {
			keep[name] = true
		}
		overrides := map[string]any{}
		for _, name := range append(folders(filepath.Join(dir, "skills")), folders(agentsSkills(in.Home))...) {
			if !keep[name] {
				overrides[name] = "off"
			}
		}
		if len(overrides) > 0 {
			out.Doc["skillOverrides"] = overrides
		}
		if agents := agentNames(filepath.Join(dir, "agents")); len(agents) > 0 {
			deny := make([]any, len(agents))
			for i, name := range agents {
				deny[i] = "Agent(" + name + ")"
			}
			out.Doc["permissions"] = map[string]any{"deny": deny}
		}
	}
	for key := range out.Doc {
		out.Overrides = append(out.Overrides, key)
	}
	sort.Strings(out.Overrides)
	return out
}

func toAny(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

func agentsSkills(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".agents", "skills")
}

// skillFolders are the folder names under <checkout>/<a>/<b>.
func skillFolders(checkout, a, b string) []string {
	if checkout == "" {
		return nil
	}
	return folders(filepath.Join(checkout, a, b))
}

// folders are the names of the folders under dir (a link to a folder
// included) that do not start with a dot, sorted; none for a dir that cannot
// be read.
func folders(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && info.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

var (
	importWord = regexp.MustCompile(`(?:^|\s)@(\S+)`)
	inlineCode = regexp.MustCompile("`[^`]*`")
)

// claudeMDExcludes is the absolute path of <dir>/CLAUDE.md followed by the
// regular files it imports with @<path>, in order of appearance: ~/ is the
// home, a relative path is relative to the file, text inside code spans and
// fenced blocks is not an import, and a path that is not a file is not listed.
// One level: the imports of an imported file are not followed.
func claudeMDExcludes(dir, home string) []string {
	main := filepath.Join(dir, "CLAUDE.md")
	out := []string{main}
	f, err := os.Open(main)
	if err != nil {
		return out
	}
	defer f.Close()
	seen := map[string]bool{main: true}
	fenced := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range importWord.FindAllStringSubmatch(inlineCode.ReplaceAllString(line, ""), -1) {
			path := strings.TrimRight(m[1], ".,;:)]}\"'")
			switch {
			case path == "":
				continue
			case path == "~" || strings.HasPrefix(path, "~/"):
				if home == "" {
					continue
				}
				path = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(path, "~")))
			case filepath.IsAbs(filepath.FromSlash(path)):
				path = filepath.FromSlash(path)
			default:
				path = filepath.Join(dir, filepath.FromSlash(path))
			}
			path = filepath.Clean(path)
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

// agentNames are the names of the agents defined in dir/*.md: the name of the
// frontmatter, else the file name without its extension; sorted, once each.
func agentNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := frontmatterName(filepath.Join(dir, e.Name()))
		if name == "" {
			name = strings.TrimSuffix(e.Name(), ".md")
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// frontmatterName is the name: value of the YAML frontmatter of a file, "" for
// none.
func frontmatterName(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return ""
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		if value, ok := strings.CutPrefix(line, "name:"); ok {
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return ""
}
