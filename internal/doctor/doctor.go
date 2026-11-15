// Package doctor is the health check behind `jitso doctor` (defect N-20): one
// function per check, each returning a Line, over ports for everything that
// touches the machine (the PATH lookup, the programs it may run, the clock,
// the filesystem, the project).
//
// It only reads. It runs `git` (version, check-ignore, ls-files) and
// `claude --version`, each with a 5 second timeout, and it looks `orca` up on
// PATH without ever running it. It writes no file, no backup and no git
// config.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
)

// Status is how a check came out.
type Status string

// The three statuses. A fail makes the command exit 3, a warning does not.
const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

// Line is the answer of one check. Check is a stable id (`hooks`,
// `skill-project`, ...), Detail one line of text.
type Line struct {
	Status Status
	Check  string
	Detail string
}

// String is the printed line, `<status> <check>: <detail>`, with the status
// padded to four characters. The detail is always one non-empty line.
func (l Line) String() string {
	return fmt.Sprintf("%-4s %s: %s", l.Status, l.Check, oneLine(l.Detail))
}

// oneLine folds line breaks into spaces: a detail may carry the text of a YAML
// error or of a program's output.
func oneLine(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	if s = strings.TrimSpace(s); s == "" {
		return "no detail"
	}
	return s
}

// Counts tallies the statuses of lines.
func Counts(lines []Line) (ok, warn, failed int) {
	for _, l := range lines {
		switch l.Status {
		case OK:
			ok++
		case Warn:
			warn++
		default:
			failed++
		}
	}
	return
}

// Summary is the last line of the output: `doctor: <n> ok, <n> warn, <n> fail`.
func Summary(lines []Line) string {
	ok, warn, failed := Counts(lines)
	return fmt.Sprintf("doctor: %d ok, %d warn, %d fail", ok, warn, failed)
}

// Result is what a program run came to.
type Result struct {
	// Stdout is what the program printed on standard output.
	Stdout string
	// Exit is its exit code (meaningless when Err is set).
	Exit int
	// Err is set when the program could not start or ran past the timeout.
	Err error
}

// Runner runs a program in dir ("" for the current folder) with standard
// input closed.
type Runner func(dir, name string, args ...string) Result

// RunTimeout is how long doctor waits for a program it runs.
const RunTimeout = 5 * time.Second

// ExecRunner is the real Runner: the program with stdin closed and
// RunTimeout to answer. stderr is discarded.
func ExecRunner(dir, name string, args ...string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), RunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	res := Result{Stdout: string(out)}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case ctx.Err() != nil:
		res.Err = fmt.Errorf("it did not answer within %s", RunTimeout)
	case errors.As(err, &exitErr):
		res.Exit = exitErr.ExitCode()
	default:
		res.Err = err
	}
	return res
}

// Ports is everything doctor reads from the machine. A nil field takes the
// real implementation, except Version, Home and Resolve, which the caller
// gives.
type Ports struct {
	// GOOS is runtime.GOOS.
	GOOS string
	// Version is the version of this binary (version.Current()).
	Version string
	// Home is $HOME.
	Home string
	// Getenv reads a variable ("" when unset).
	Getenv func(string) string
	// Executable is os.Executable.
	Executable func() (string, error)
	// LookPath is exec.LookPath.
	LookPath func(string) (string, error)
	// Run runs git and claude.
	Run Runner
	// Now is the clock.
	Now func() time.Time
	// Resolve finds the project of the working folder (Context.Project).
	Resolve func() (project.Root, error)

	ReadFile func(string) ([]byte, error)
	Stat     func(string) (fs.FileInfo, error)
	Lstat    func(string) (fs.FileInfo, error)
	Readlink func(string) (string, error)
	ReadDir  func(string) ([]fs.DirEntry, error)
}

func (p Ports) withDefaults() Ports {
	if p.GOOS == "" {
		p.GOOS = "linux"
	}
	if p.Getenv == nil {
		p.Getenv = func(string) string { return "" }
	}
	if p.Executable == nil {
		p.Executable = os.Executable
	}
	if p.LookPath == nil {
		p.LookPath = exec.LookPath
	}
	if p.Run == nil {
		p.Run = ExecRunner
	}
	if p.Now == nil {
		p.Now = time.Now
	}
	if p.Resolve == nil {
		p.Resolve = func() (project.Root, error) { return project.Root{}, fail.Env("no project") }
	}
	if p.ReadFile == nil {
		p.ReadFile = os.ReadFile
	}
	if p.Stat == nil {
		p.Stat = os.Stat
	}
	if p.Lstat == nil {
		p.Lstat = os.Lstat
	}
	if p.Readlink == nil {
		p.Readlink = os.Readlink
	}
	if p.ReadDir == nil {
		p.ReadDir = os.ReadDir
	}
	return p
}

// Run is every check, in the order they print. Outside a git checkout the
// project checks are left out (`project` fails and says so); the machine and
// user-scope checks still run.
func Run(p Ports) []Line {
	p = p.withDefaults()
	var lines []Line
	add := func(l ...Line) { lines = append(lines, l...) }

	binary, self := checkBinary(p)
	add(binary, checkPath(p, self), checkGit(p), checkShell(p), checkDispatcher(p, self), checkOrca(p), checkClaude(p), checkUserConfig(p))

	proj, root, ok := checkProject(p)
	add(proj)
	var projectSkill skillFound
	if ok {
		c := &projectChecks{p: p, root: root}
		add(c.catalog(), c.mcp(), c.state(), c.registry(), c.drafts(), c.backups(), c.hooks())
		var line Line
		line, projectSkill = checkSkill(p, "project", root.Checkout, nil)
		add(line)
	}
	userSkill, _ := checkSkill(p, "user", p.Home, &projectSkill)
	add(userSkill)
	if ok {
		add(checkInstructions(p, root))
	}
	add(checkInstructionsUser(p))
	return lines
}

// message is the text of an error without the "jitso: " prefix the CLI adds.
func message(err error) string {
	var fe *fail.Error
	if errors.As(err, &fe) && fe.Msg != "" {
		return fe.Msg
	}
	return unwrap(err).Error()
}

// describe is message for an error that does not name its file itself (a
// read error of a path the caller never shows): the whole text is kept.
func describe(err error) string {
	var fe *fail.Error
	if errors.As(err, &fe) && fe.Msg != "" {
		return fe.Msg
	}
	return err.Error()
}

// unwrap drops the operation and path a *fs.PathError repeats.
func unwrap(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// plural is "<n> <noun>", the noun in the plural unless n is 1.
func plural(n int, noun string) string {
	switch {
	case n == 1:
	case strings.HasSuffix(noun, "ch") || strings.HasSuffix(noun, "sh") || strings.HasSuffix(noun, "s"):
		noun += "es"
	default:
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}

// firstLine is the first non-blank line of text, trimmed.
func firstLine(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// slash is a path in slash form.
func slash(p string) string { return filepath.ToSlash(p) }
