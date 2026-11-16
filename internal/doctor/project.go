package doctor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// mcpFile is the repo-level MCP server table, at the primary checkout.
const mcpFile = ".mcp.json"

// DraftWarnAge is how old the oldest draft may be before it is worth a
// warning: a draft is meant to be saved or discarded soon.
const DraftWarnAge = 7 * 24 * time.Hour

// projectChecks are the checks that need the project of the working folder.
type projectChecks struct {
	p    Ports
	root project.Root

	cat    *catalog.Catalog
	catErr bool
}

// gitSays runs one git question in the primary checkout and reports the exit
// code; ok is false when git could not answer (it did not start, timed out or
// failed in a way that is not a plain no).
func (c *projectChecks) gitSays(dir string, args ...string) (exit int, ok bool) {
	res := c.p.Run(dir, "git", args...)
	if res.Err != nil || res.Exit > 1 {
		return 0, false
	}
	return res.Exit, true
}

// catalog loads the layered catalog the way every catalog command does: the
// project's own files keep the whole-catalog rule, outer files that are
// invalid here are skipped and named.
func (c *projectChecks) catalog() Line {
	res, err := catalog.Scan(os.DirFS(c.root.Primary), c.root.Primary, c.p.Home)
	if err == nil {
		c.cat, err = res.Catalog()
	}
	if err != nil {
		c.catErr = true
		return Line{Fail, "catalog", describe(err)}
	}
	detail := fmt.Sprintf("%s, %s, %d launchable", catalog.ConfigPath, plural(len(c.cat.Builds), "build"), len(c.cat.Launchable()))
	if len(c.cat.Skipped) == 0 {
		return Line{OK, "catalog", detail}
	}
	parts := []string{detail}
	for _, s := range c.cat.Skipped {
		parts = append(parts, fmt.Sprintf("skipped %s %s (%s)", s.Layer, s.Display, strings.Join(s.Problems, ", ")))
	}
	return Line{Warn, "catalog", strings.Join(parts, "; ")}
}

// mcp checks that the servers the launchable builds list are in .mcp.json.
func (c *projectChecks) mcp() Line {
	if c.cat == nil {
		return Line{Warn, "mcp", "not checked: the catalog did not load"}
	}
	listed := map[string][]string{} // server -> build ids
	for _, b := range c.cat.Launchable() {
		for _, s := range b.MCP {
			listed[s] = append(listed[s], b.ID)
		}
	}
	file := filepath.Join(c.root.Primary, mcpFile)
	data, err := c.p.ReadFile(file)
	absent := errors.Is(err, fs.ErrNotExist)
	var servers map[string]any
	var problem string
	switch {
	case absent:
	case err != nil:
		problem = mcpFile + " cannot be read: " + message(err)
	default:
		var v any
		if jerr := json.Unmarshal(data, &v); jerr != nil {
			problem = mcpFile + " is not valid JSON: " + jerr.Error()
		} else if v == nil {
			problem = mcpFile + " is null, not an object with mcpServers"
		} else {
			top, _ := v.(map[string]any)
			servers, _ = top["mcpServers"].(map[string]any)
		}
	}

	if len(listed) == 0 {
		switch {
		case problem != "":
			return Line{Warn, "mcp", problem}
		case absent:
			return Line{OK, "mcp", "no build lists an MCP server"}
		}
		return Line{OK, "mcp", mcpFile + ": " + plural(len(servers), "server")}
	}
	names := make([]string, 0, len(listed))
	for n := range listed {
		names = append(names, n)
	}
	sort.Strings(names)
	switch {
	case absent:
		return Line{Fail, "mcp", mcpFile + " is missing, and builds list MCP servers (" + strings.Join(names, ", ") + ")"}
	case problem != "":
		return Line{Fail, "mcp", problem}
	}
	var unknown []string
	for _, n := range names {
		if _, ok := servers[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return Line{Fail, "mcp", mcpFile + " does not define " + strings.Join(unknown, ", ") + ", which builds list"}
	}
	return Line{OK, "mcp", mcpFile + ": " + plural(len(servers), "server")}
}

// state checks that the local state folder stays out of commits.
func (c *projectChecks) state() Line {
	exit, ok := c.gitSays(c.root.Primary, "check-ignore", "-q", ".jitso/"+registry.FileName)
	switch {
	case !ok:
		return Line{Warn, "state", "git could not say whether .jitso/ is ignored"}
	case exit == 0:
		return Line{OK, "state", ".jitso/ is ignored by git"}
	}
	return Line{Warn, "state", ".jitso/ is not ignored by git: add .jitso/ to .gitignore"}
}

// registry reads the launch log the way every reader does: a line that is not
// a valid event is skipped, so the check counts them, the v1 lines apart.
func (c *projectChecks) registry() Line {
	file := filepath.Join(c.root.StateDir(), registry.FileName)
	shown := ".jitso/" + registry.FileName
	data, err := c.p.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return Line{OK, "registry", "no launches yet"}
	}
	if err != nil {
		return Line{Fail, "registry", shown + " cannot be read: " + message(err)}
	}
	lines := 0
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			lines++
		}
	}
	events := registry.Parse(data)
	launches := registry.Fold(events)
	invalid := lines - len(events)
	v1 := registry.CountV1(data)
	if len(launches) == 0 && invalid == 0 {
		return Line{OK, "registry", "no launches yet"}
	}
	running := 0
	for i := range launches {
		if !launches[i].Ended() {
			running++
		}
	}
	detail := fmt.Sprintf("%s: %s, %d running", shown, plural(len(launches), "launch"), running)
	if invalid == 0 {
		return Line{OK, "registry", detail}
	}
	// A v1 line is an invalid line too, but it gets its own clause: it is not
	// corruption, it is a line written before the log moved to v: 2.
	if v1 > 0 {
		detail += fmt.Sprintf("; %s skipped (written before the log moved to v: %d)", plural(v1, "v1 line"), registry.Version)
	}
	if other := invalid - v1; other > 0 {
		detail += fmt.Sprintf("; %s skipped", plural(other, "invalid line"))
	}
	return Line{Warn, "registry", detail}
}

// drafts counts the just-in-time builds waiting for a decision.
func (c *projectChecks) drafts() Line {
	ids, err := drafts.List(c.root.Primary)
	if err != nil {
		return Line{Warn, "drafts", ".jitso/drafts/ cannot be read: " + message(err)}
	}
	if len(ids) == 0 {
		return Line{OK, "drafts", "no drafts"}
	}
	var warns []string
	if exit, ok := c.gitSays(c.root.Primary, "check-ignore", "-q", ".jitso/drafts/"+ids[0]+drafts.ExtensionYAML); ok && exit == 1 {
		warns = append(warns, ".jitso/drafts/ is not ignored by git")
	}
	var oldest time.Duration
	for _, id := range ids {
		info, err := c.p.Stat(drafts.File(c.root.Primary, id))
		if err != nil {
			continue
		}
		if age := c.p.Now().Sub(info.ModTime()); age > oldest {
			oldest = age
		}
	}
	count := plural(len(ids), "draft")
	if oldest > DraftWarnAge {
		warns = append(warns, fmt.Sprintf("%s, the oldest is %s old", count, plural(int(oldest/(24*time.Hour)), "day")))
	}
	if len(warns) == 0 {
		return Line{OK, "drafts", count}
	}
	return Line{Warn, "drafts", strings.Join(warns, "; ")}
}

// backups counts what the adoption commands saved before changing a file.
func (c *projectChecks) backups() Line {
	entries, err := c.p.ReadDir(filepath.Join(c.root.StateDir(), "backups"))
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Line{Warn, "backups", ".jitso/backups/ cannot be read: " + message(err)}
	}
	if n == 0 {
		return Line{OK, "backups", "no backups"}
	}
	return Line{OK, "backups", plural(n, "backup") + " in .jitso/backups/"}
}
