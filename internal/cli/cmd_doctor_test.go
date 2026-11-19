package cli

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// doctorRun runs `jitso doctor ...` in cwd. PATH holds git alone, so the run
// never reaches a real claude or orca on the machine of the test.
func doctorRun(t *testing.T, cwd string, args ...string) (int, string, string) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	// A temp folder inside some repository must not become its project.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(cwd))
	var out, errOut bytes.Buffer
	env := map[string]string{"HOME": filepath.Join(cwd, "home")}
	code := Run(append([]string{"doctor"}, args...), System{
		Stdin:      strings.NewReader(""),
		Stdout:     &out,
		Stderr:     &errOut,
		LookupEnv:  func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		Getwd:      func() (string, error) { return cwd, nil },
		Executable: os.Executable,
		Now:        func() time.Time { return time.Date(2026, 11, 15, 7, 34, 55, 0, time.UTC) },
	})
	return code, out.String(), errOut.String()
}

func TestDoctorUsageAnswers(t *testing.T) {
	cwd := t.TempDir()
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{"--help", []string{"--help"}, 0, doctorUsage + "\n", ""},
		{"-h", []string{"-h"}, 0, doctorUsage + "\n", ""},
		{"a word", []string{"x"}, 2, "", "jitso doctor: takes no arguments\n"},
		{"a flag", []string{"--json"}, 2, "", "jitso doctor: takes no arguments\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := doctorRun(t, cwd, tt.args...)
			if code != tt.code || out != tt.stdout || errOut != tt.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
}

// tree lists every path under root except .git, with its mode and size.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		info, _ := d.Info()
		rel, _ := filepath.Rel(root, p)
		out = append(out, rel+" "+info.Mode().String()+" "+info.ModTime().String())
		return nil
	})
	sort.Strings(out)
	return out
}

func TestDoctorOutsideGit(t *testing.T) {
	cwd := t.TempDir()
	_, out, errOut := doctorRun(t, cwd)
	if errOut != "" {
		t.Errorf("stderr %q", errOut)
	}
	if strings.Contains(out, "fail project:") {
		t.Errorf("outside a checkout the project is a warning, not a failure:\n%s", out)
	}
	for _, want := range []string{"\nwarn project: ", " is not inside a git checkout; the project checks are skipped\n", "ok   binary: ", "doctor: "} {
		if !strings.Contains("\n"+out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "catalog:") {
		t.Errorf("the project checks are left out:\n%s", out)
	}
}

func TestDoctorInAProjectWritesNothing(t *testing.T) {
	cwd := t.TempDir()
	if err := exec.Command("git", "-C", cwd, "init", "-q").Run(); err != nil {
		t.Skip("git init failed")
	}
	before := tree(t, cwd)
	code, out, errOut := doctorRun(t, cwd)
	// No jitso/jitso.yaml here, so the catalog check fails: exit 3 with nothing on stderr.
	if code != 3 || errOut != "" {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if !regexp.MustCompile(`(?m)^fail catalog: jitso/jitso\.yaml is missing`).MatchString(out) {
		t.Errorf("stdout:\n%s", out)
	}
	if !regexp.MustCompile(`(?m)^doctor: [0-9]+ ok, [0-9]+ warn, [1-9][0-9]* fail$`).MatchString(out) {
		t.Errorf("summary line missing:\n%s", out)
	}
	if after := tree(t, cwd); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("doctor changed the project:\nbefore %v\nafter  %v", before, after)
	}
}
