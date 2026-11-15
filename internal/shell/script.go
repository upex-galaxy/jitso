package shell

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/upex-galaxy/jitso/internal/harness"
)

var (
	programName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	envName     = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// RunScript is the text of run.sh: the shebang, one `export NAME='value'`
// line per env entry in order, and one exec line with every word quoted. The
// program (argv[0]) must be a plain program name and every env name must
// match ^[A-Z_][A-Z0-9_]*$, so neither ever needs escaping. The env program is
// not used, so the argv stays exactly as the adapter built it (S2).
func RunScript(env []harness.EnvVar, argv []string) (string, error) {
	if len(argv) == 0 || !programName.MatchString(argv[0]) {
		program := "undefined"
		if len(argv) > 0 {
			program = argv[0]
		}
		return "", errors.New(`"` + program + `" is not a plain program name`)
	}
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	for _, v := range env {
		if !envName.MatchString(v.Name) {
			return "", errors.New(`"` + v.Name + `" is not a plain variable name`)
		}
		value, err := Quote(v.Value)
		if err != nil {
			return "", err
		}
		b.WriteString("export " + v.Name + "=" + value + "\n")
	}
	words := make([]string, len(argv))
	for i, word := range argv {
		q, err := Quote(word)
		if err != nil {
			return "", err
		}
		words[i] = q
	}
	b.WriteString("exec " + strings.Join(words, " ") + "\n")
	return b.String(), nil
}

// CommandFor is the --command value of `orca terminal create`: the absolute
// run.sh path as one quoted word.
func CommandFor(runSh string) (string, error) {
	if !filepath.IsAbs(runSh) {
		return "", errors.New(`the run.sh path must be absolute, got "` + runSh + `"`)
	}
	return Quote(runSh)
}

// TerminalTitle is the mark a launch's Orca terminal carries so the launch
// can be found again: [jitso:<id>]. It fails on a value that is not a launch id.
func TerminalTitle(id string) (string, error) {
	if !launchID.MatchString(id) {
		return "", errors.New("not a launch id")
	}
	return "[jitso:" + id + "]", nil
}
