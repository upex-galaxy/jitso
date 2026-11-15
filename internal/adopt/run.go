package adopt

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

// Options is how one command run is configured.
type Options struct {
	// Command is the command as typed after jitso: "install skill",
	// "setup hook". It starts the status lines (jitso <cmd>: done).
	Command string
	// Scope is "project" or "user" (for the backup manifest).
	Scope string
	// Version is the jitso version (for the backup manifest).
	Version string
	// Root is the absolute scope root: the checkout for project scope, HOME
	// for user scope. Every plan path lies under it.
	Root string
	// BackupsDir is the absolute folder that holds the backups.
	BackupsDir string
	// DryRun is --dry-run and Yes is --yes. A dry run wins.
	DryRun bool
	Yes    bool
	// Display renders an absolute path for the lines that print a backup
	// location (project scope: relative to the checkout; user scope: ~/...);
	// nil prints the path as is.
	Display func(string) string
}

// Ports are the things a run reads from or writes to besides the files.
type Ports struct {
	// Stdin is read for one line when a terminal is asked a question; nil is
	// an input that has ended.
	Stdin io.Reader
	// Stdout takes the preview, the prompt and the progress lines.
	Stdout io.Writer
	// Terminal is whether Stdin is a terminal (internal/tty); only then is the
	// question asked.
	Terminal bool
	// Now and Rand make the backup id. Nil means the real clock and crypto/rand.
	Now  func() time.Time
	Rand io.Reader
}

// Run is the common flow of the adoption commands:
//
//  1. an empty plan prints `jitso <cmd>: nothing to do (<why>)` and ends;
//  2. the preview goes to stdout (Preview);
//  3. the decision (Confirm): `--dry-run` prints `jitso <cmd>: dry run,
//     nothing written`; `--yes` applies; on a terminal the question is asked;
//     with no terminal and no `--yes`, or an answer that is not yes, the run
//     fails with a rejection (exit 2) and writes nothing;
//  4. Apply, the plan's notes, and the line `jitso <cmd>: done`.
//
// The error is already a package fail error: exit 2 for a refusal, exit 3 for
// a bad plan or a failed write.
func Run(opts Options, plan Plan, p Ports) error {
	if err := plan.Validate(opts.Root); err != nil {
		return err
	}
	if plan.Empty() {
		if plan.Why == "" {
			fmt.Fprintf(p.Stdout, "jitso %s: nothing to do\n", opts.Command)
		} else {
			fmt.Fprintf(p.Stdout, "jitso %s: nothing to do (%s)\n", opts.Command, plan.Why)
		}
		return nil
	}
	Preview(p.Stdout, plan)

	c := Confirm{DryRun: opts.DryRun, Yes: opts.Yes, Terminal: p.Terminal}
	d := c.Before()
	if d == DecideAsk {
		io.WriteString(p.Stdout, Prompt)
		line, eof := readLine(p.Stdin)
		d = Reply(line, eof)
	}
	switch d {
	case DecideDryRun:
		fmt.Fprintf(p.Stdout, "jitso %s: dry run, nothing written\n", opts.Command)
		return nil
	case DecideApply:
	default:
		return d.Err()
	}

	spec := BackupSpec{
		Dir:     opts.BackupsDir,
		Root:    opts.Root,
		Command: opts.Command,
		Scope:   opts.Scope,
		Version: opts.Version,
		Now:     p.Now,
		Rand:    p.Rand,
		Display: opts.Display,
	}
	if spec.Now == nil {
		spec.Now = time.Now
	}
	if spec.Rand == nil {
		spec.Rand = rand.Reader
	}
	if err := Apply(spec, plan.Effective(), p.Stdout); err != nil {
		return err
	}
	for _, note := range plan.Notes {
		fmt.Fprintln(p.Stdout, note)
	}
	fmt.Fprintf(p.Stdout, "jitso %s: done\n", opts.Command)
	return nil
}

// readLine reads one line from r. eof is true when the input ended: before
// any byte, or with a last line that had no newline.
func readLine(r io.Reader) (line string, eof bool) {
	if r == nil {
		return "", true
	}
	line, err := bufio.NewReader(r).ReadString('\n')
	return line, err != nil
}

// Preview prints what the plan would change, for each effective change in
// plan order: the plan's header lines, then a unified diff (no unchanged line
// for a Sensitive file, where only the lines jitso wrote are printed; three
// lines of context otherwise), or one line for a
// Brief change: `new file <path> (<n> lines)`, `delete <path>`; and for the
// kinds that are not file content `link <path> -> <target>`, `unlink <path>`
// and `new folder <path>`. A Prune change is not shown.
func Preview(w io.Writer, plan Plan) {
	if plan.Empty() {
		return
	}
	for _, h := range plan.Header {
		fmt.Fprintln(w, h)
	}
	for _, c := range plan.Effective() {
		if c.Prune {
			continue
		}
		io.WriteString(w, previewOne(c))
	}
}

func previewOne(c Change) string {
	switch c.Kind {
	case KindSymlink:
		if c.Target == "" {
			return "unlink " + c.Display + "\n"
		}
		return "link " + c.Display + " -> " + c.Target + "\n"
	case KindMkdir:
		return "new folder " + c.Display + "\n"
	case KindDelete:
		if c.Brief || c.Old == nil {
			return "delete " + c.Display + "\n"
		}
		return diffOf(c, c.Old, nil)
	}
	if c.Brief && !c.Existed() {
		return fmt.Sprintf("new file %s (%d lines)\n", c.Display, LineCount(c.New))
	}
	content := c.New
	if content == nil {
		content = []byte{}
	}
	return diffOf(c, c.Old, content)
}

func diffOf(c Change, old, new []byte) string {
	if c.Sensitive {
		return UnifiedSensitive(c.Display, old, new, c.Written)
	}
	return UnifiedWith(c.Display, old, new, 3, FallbackHunk)
}
