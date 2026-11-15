package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"jitso-contract-runner/internal/expect"
	"jitso-contract-runner/internal/jsonx"
)

// CaseFile is the file name of a case.
const CaseFile = "case.json"

// StatusKeys are the implementations a case may declare a status for.
var StatusKeys = []string{"ts", "go"}

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var (
	expansionIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	worktreeNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// Discover returns every folder under root that holds a case.json, sorted.
func Discover(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == CaseFile {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	sort.Strings(dirs)
	return dirs, err
}

// Load reads and validates the case in dir. Every check that does not need a sandbox runs
// here, once per expansion, with stub values for the built-ins and the captures: unknown
// keys, types, placeholders, regexes, matcher shapes, file map paths.
func Load(casesRoot, dir, fixturesRoot string) (*Case, error) {
	rel, err := filepath.Rel(casesRoot, dir)
	if err != nil {
		return nil, err
	}
	wantID := filepath.ToSlash(rel)
	data, err := os.ReadFile(filepath.Join(dir, CaseFile))
	if err != nil {
		return nil, err
	}
	tree, err := jsonx.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	obj, err := jsonx.Object(tree, "case")
	if err != nil {
		return nil, err
	}
	if err := jsonx.Keys(obj, "case", "id", "title", "source", "tags", "status", "defect", "intent", "project", "home", "orca", "claude", "for_each", "steps"); err != nil {
		return nil, err
	}
	c := &Case{Dir: dir, Base: "default", Status: map[string]string{}, orca: obj["orca"], claude: obj["claude"]}
	if err := c.readMeta(obj, wantID); err != nil {
		return nil, err
	}
	if err := c.readSandbox(obj, fixturesRoot); err != nil {
		return nil, err
	}
	if err := c.readForEach(obj["for_each"]); err != nil {
		return nil, err
	}
	if err := c.readSteps(obj["steps"]); err != nil {
		return nil, err
	}
	return c, c.validate()
}

func (c *Case) readMeta(obj map[string]any, wantID string) error {
	var err error
	if c.ID, err = jsonx.String(obj["id"], "id"); err != nil {
		return err
	}
	if c.ID != wantID {
		return fmt.Errorf("id %q must equal the folder path %q", c.ID, wantID)
	}
	if c.Title, err = jsonx.String(obj["title"], "title"); err != nil {
		return err
	}
	if c.Title == "" {
		return errors.New("title: must not be empty")
	}
	if v, ok := obj["source"]; ok {
		if c.Source, err = jsonx.Strings(v, "source"); err != nil {
			return err
		}
	}
	if v, ok := obj["tags"]; ok {
		if c.Tags, err = jsonx.Strings(v, "tags"); err != nil {
			return err
		}
		for _, t := range c.Tags {
			if !tagPattern.MatchString(t) {
				return fmt.Errorf("tags: %q is not a lowercase tag", t)
			}
		}
	}
	if v, ok := obj["status"]; ok {
		if c.Status, err = jsonx.StringMap(v, "status"); err != nil {
			return err
		}
	}
	xfail := false
	for key, s := range c.Status {
		if !contains(StatusKeys, key) {
			return fmt.Errorf("status: unknown implementation %q (known: %v)", key, StatusKeys)
		}
		if s != StatusPass && s != StatusXfail {
			return fmt.Errorf("status.%s: want pass or xfail, got %q", key, s)
		}
		xfail = xfail || s == StatusXfail
	}
	for _, f := range []struct {
		key string
		dst *string
	}{{"defect", &c.Defect}, {"intent", &c.Intent}} {
		if v, ok := obj[f.key]; ok {
			if *f.dst, err = jsonx.String(v, f.key); err != nil {
				return err
			}
		}
	}
	if xfail && (c.Defect == "" || c.Intent == "") {
		return errors.New("an xfail status needs both defect and intent")
	}
	return nil
}

func (c *Case) readSandbox(obj map[string]any, fixturesRoot string) error {
	if v, ok := obj["project"]; ok {
		p, err := jsonx.Object(v, "project")
		if err != nil {
			return err
		}
		if err := jsonx.Keys(p, "project", "base", "files"); err != nil {
			return err
		}
		if b, ok := p["base"]; ok {
			if c.Base, err = jsonx.String(b, "project.base"); err != nil {
				return err
			}
		}
		c.projectFiles = p["files"]
	}
	if !filepath.IsLocal(c.Base) {
		return fmt.Errorf("project.base: %q is not a fixture folder name", c.Base)
	}
	if st, err := os.Stat(filepath.Join(fixturesRoot, c.Base)); err != nil || !st.IsDir() {
		return fmt.Errorf("project.base: fixture %q not found under %s", c.Base, fixturesRoot)
	}
	if v, ok := obj["home"]; ok {
		h, err := jsonx.Object(v, "home")
		if err != nil {
			return err
		}
		if err := jsonx.Keys(h, "home", "files"); err != nil {
			return err
		}
		c.homeFiles = h["files"]
	}
	return nil
}

func (c *Case) readForEach(v any) error {
	if v == nil {
		return nil
	}
	obj, err := jsonx.Object(v, "for_each")
	if err != nil {
		return err
	}
	if err := jsonx.Keys(obj, "for_each", "var", "values", "vars", "rows", "ids"); err != nil {
		return err
	}
	_, single := obj["var"]
	_, multi := obj["vars"]
	fe := &ForEach{}
	switch {
	case single && !multi:
		if _, ok := obj["rows"]; ok {
			return errors.New("for_each: var goes with values, vars with rows")
		}
		name, err := jsonx.String(obj["var"], "for_each.var")
		if err != nil {
			return err
		}
		values, err := jsonx.Strings(obj["values"], "for_each.values")
		if err != nil {
			return err
		}
		fe.Vars = []string{name}
		for _, v := range values {
			fe.Rows = append(fe.Rows, []string{v})
		}
	case multi && !single:
		if _, ok := obj["values"]; ok {
			return errors.New("for_each: var goes with values, vars with rows")
		}
		if fe.Vars, err = jsonx.Strings(obj["vars"], "for_each.vars"); err != nil {
			return err
		}
		if len(fe.Vars) == 0 {
			return errors.New("for_each.vars: give at least one name")
		}
		rows, err := jsonx.List(obj["rows"], "for_each.rows")
		if err != nil {
			return err
		}
		for i, r := range rows {
			row, err := jsonx.Strings(r, fmt.Sprintf("for_each.rows[%d]", i))
			if err != nil {
				return err
			}
			if len(row) != len(fe.Vars) {
				return fmt.Errorf("for_each.rows[%d]: %d values for %d vars", i, len(row), len(fe.Vars))
			}
			fe.Rows = append(fe.Rows, row)
		}
	default:
		return errors.New("for_each: give either var and values, or vars and rows")
	}
	seen := map[string]bool{}
	for _, name := range fe.Vars {
		if !namePattern.MatchString(name) || isReserved(name) || seen[name] {
			return fmt.Errorf("for_each: %q is not a free placeholder name (or is listed twice)", name)
		}
		seen[name] = true
	}
	if len(fe.Rows) == 0 {
		return errors.New("for_each: give at least one value")
	}
	if raw, ok := obj["ids"]; ok {
		if fe.IDs, err = jsonx.Strings(raw, "for_each.ids"); err != nil {
			return err
		}
		if len(fe.IDs) != len(fe.Rows) {
			return fmt.Errorf("for_each.ids: %d ids for %d expansions", len(fe.IDs), len(fe.Rows))
		}
		used := map[string]bool{}
		for _, id := range fe.IDs {
			if !expansionIDPattern.MatchString(id) || used[id] {
				return fmt.Errorf("for_each.ids: %q is not a plain unique name", id)
			}
			used[id] = true
		}
	}
	c.ForEach = fe
	return nil
}

func (c *Case) readSteps(v any) error {
	list, err := jsonx.List(v, "steps")
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errors.New("steps: give at least one step")
	}
	for i, item := range list {
		step, err := jsonx.Object(item, fmt.Sprintf("steps[%d]", i))
		if err != nil {
			return err
		}
		c.steps = append(c.steps, step)
		if raw, ok := step[expect.KindGitWorktree]; ok {
			name, err := worktreeName(raw, fmt.Sprintf("steps[%d].git_worktree", i))
			if err != nil {
				return err
			}
			if contains(c.worktrees, name) {
				return fmt.Errorf("steps[%d].git_worktree: %q is created twice", i, name)
			}
			c.worktrees = append(c.worktrees, name)
		}
	}
	return nil
}

// worktreeName reads the literal name of a git_worktree step (no placeholder: it names one).
func worktreeName(v any, what string) (string, error) {
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return "", err
	}
	if err := jsonx.Keys(obj, what, "name"); err != nil {
		return "", err
	}
	name, err := jsonx.String(obj["name"], what+".name")
	if err != nil {
		return "", err
	}
	if !worktreeNamePattern.MatchString(name) {
		return "", fmt.Errorf("%s.name: %q is not a plain name ([A-Za-z0-9_])", what, name)
	}
	return name, nil
}

// validate builds every expansion once with stub values, so a bad case fails at load.
func (c *Case) validate() error {
	captures, err := c.captureNames()
	if err != nil {
		return err
	}
	builtins := Vars{VarProject: "/stub/project", VarHome: "/stub/home", VarBin: "/stub/bin", VarJitso: "/stub/jitso"}
	for _, name := range c.worktrees {
		builtins[WorktreeVar(name)] = "/stub/worktrees/" + name
	}
	stubCaptures := Vars{}
	for name := range captures {
		stubCaptures[name] = "STUB_" + name
	}
	for _, x := range c.Expand() {
		where := ""
		if c.ForEach != nil {
			where = fmt.Sprintf("[%d] ", x.Index)
		}
		setupVars := x.Bind(builtins)
		if _, err := x.Setup(setupVars); err != nil {
			return fmt.Errorf("%s%w", where, err)
		}
		stepVars := setupVars.With(stubCaptures)
		for i := range c.steps {
			a, err := x.Action(i, stepVars)
			if err != nil {
				return fmt.Errorf("%ssteps[%d]: %w", where, i, err)
			}
			if _, err := x.Expect(i, a.Kind, stepVars); err != nil {
				return fmt.Errorf("%ssteps[%d]: %w", where, i, err)
			}
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
