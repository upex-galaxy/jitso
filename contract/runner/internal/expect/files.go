package expect

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"jitso-contract-runner/internal/jsonx"
)

// files (SPEC 7.2): existence, the owner execute bit, content and JSON checks per path.
func init() {
	register("files", allKinds, func(v any, what string) (Check, error) {
		list, err := jsonx.List(v, what)
		if err != nil {
			return nil, err
		}
		checks := make([]fileCheck, len(list))
		for i, item := range list {
			fc, err := parseFileCheck(item, fmt.Sprintf("%s[%d]", what, i))
			if err != nil {
				return nil, err
			}
			checks[i] = fc
		}
		return checkFunc(func(obs *Observation) []Failure {
			var out []Failure
			for _, fc := range checks {
				out = append(out, fc.eval(obs.Project)...)
			}
			return out
		}), nil
	})
}

type fileCheck struct {
	path       string
	exists     bool
	executable *bool
	text       []TextMatcher
	json       []JSONCheck
	// entries, when set, checks a directory: its entry count, or text matchers on the
	// sorted entry names joined by newlines.
	hasEntries   bool
	entriesCount int
	entriesText  []TextMatcher
}

func parseFileCheck(v any, what string) (fileCheck, error) {
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return fileCheck{}, err
	}
	if err := jsonx.Keys(obj, what, "path", "exists", "executable", "contains", "not_contains", "regex", "equals", "text", "json", "entries"); err != nil {
		return fileCheck{}, err
	}
	fc := fileCheck{exists: true}
	if fc.path, err = jsonx.String(obj["path"], what+".path"); err != nil {
		return fileCheck{}, err
	}
	if raw, ok := obj["exists"]; ok {
		if fc.exists, err = jsonx.Bool(raw, what+".exists"); err != nil {
			return fileCheck{}, err
		}
	}
	if raw, ok := obj["executable"]; ok {
		b, err := jsonx.Bool(raw, what+".executable")
		if err != nil {
			return fileCheck{}, err
		}
		fc.executable = &b
	}
	// contains / not_contains / regex are lists of strings; equals is one string.
	for _, key := range []string{"contains", "not_contains", "regex"} {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		items, err := jsonx.Strings(raw, what+"."+key)
		if err != nil {
			return fileCheck{}, err
		}
		for i, s := range items {
			m, err := textMatchers[key](s, fmt.Sprintf("%s.%s[%d]", what, key, i))
			if err != nil {
				return fileCheck{}, err
			}
			fc.text = append(fc.text, m)
		}
	}
	if raw, ok := obj["equals"]; ok {
		m, err := textMatchers["equals"](raw, what+".equals")
		if err != nil {
			return fileCheck{}, err
		}
		fc.text = append(fc.text, m)
	}
	if raw, ok := obj["text"]; ok {
		matchers, err := ParseTextMatchers(raw, what+".text")
		if err != nil {
			return fileCheck{}, err
		}
		fc.text = append(fc.text, matchers...)
	}
	if raw, ok := obj["json"]; ok {
		if fc.json, err = ParseJSONChecks(raw, what+".json"); err != nil {
			return fileCheck{}, err
		}
	}
	if raw, ok := obj["entries"]; ok {
		if err := fc.parseEntries(raw, what+".entries"); err != nil {
			return fileCheck{}, err
		}
		if len(fc.text) > 0 || len(fc.json) > 0 {
			return fileCheck{}, fmt.Errorf("%s: entries checks a directory and takes no content check", what)
		}
	}
	if !fc.exists && (fc.executable != nil || len(fc.text) > 0 || len(fc.json) > 0 || fc.hasEntries) {
		return fileCheck{}, fmt.Errorf("%s: exists:false takes no other check", what)
	}
	return fc, nil
}

func (fc fileCheck) eval(project string) []Failure {
	path := fc.path
	if !filepath.IsAbs(path) {
		path = filepath.Join(project, path)
	}
	label := "file " + fc.path
	info, err := os.Lstat(path)
	exists := err == nil
	if exists != fc.exists {
		if fc.exists {
			return fail(label, "exists", "does not exist")
		}
		return fail(label, "does not exist", "exists")
	}
	if !exists {
		return nil
	}
	var out []Failure
	if fc.hasEntries {
		out = append(out, fc.evalEntries(label, path)...)
	}
	if fc.executable != nil && runtime.GOOS != "windows" { // Windows has no execute bit
		st, err := os.Stat(path)
		isExec := err == nil && st.Mode()&0o100 != 0
		if isExec != *fc.executable {
			out = append(out, Failure{What: label + " executable", Expected: fmt.Sprint(*fc.executable), Actual: fmt.Sprintf("%v (mode %v)", isExec, info.Mode())})
		}
	}
	if len(fc.text) == 0 && len(fc.json) == 0 {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return append(out, Failure{What: label, Expected: "readable", Actual: err.Error()})
	}
	out = append(out, evalText(label, fc.text, string(data))...)
	if len(fc.json) > 0 {
		doc, err := jsonx.Decode(data)
		if err != nil {
			return append(out, Failure{What: label + " json", Expected: "valid JSON", Actual: err.Error()})
		}
		out = append(out, evalJSON(label+" json", fc.json, doc)...)
	}
	return out
}

// parseEntries reads `{"count": n}` or a list of text matchers.
func (fc *fileCheck) parseEntries(v any, what string) error {
	fc.hasEntries, fc.entriesCount = true, -1
	if obj, ok := v.(map[string]any); ok {
		if err := jsonx.Keys(obj, what, "count"); err != nil {
			return err
		}
		n, err := jsonx.Int(obj["count"], what+".count")
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("%s.count: want 0 or more", what)
		}
		fc.entriesCount = n
		return nil
	}
	matchers, err := ParseTextMatchers(v, what)
	if err != nil {
		return fmt.Errorf("%s: want {\"count\": n} or a list of text matchers: %w", what, err)
	}
	fc.entriesText = matchers
	return nil
}

func (fc fileCheck) evalEntries(label, path string) []Failure {
	list, err := os.ReadDir(path)
	if err != nil {
		return fail(label+" entries", "a readable directory", err.Error())
	}
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = e.Name()
	}
	if fc.entriesCount >= 0 && len(names) != fc.entriesCount {
		return fail(label+" entries count", fmt.Sprint(fc.entriesCount), fmt.Sprintf("%d %s", len(names), jsonx.Compact(names)))
	}
	return evalText(label+" entries", fc.entriesText, strings.Join(names, "\n"))
}
