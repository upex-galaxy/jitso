package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// scan builds the scan result `build validate` reports from, by hand: a
// repo file, an invalid repo file, a second repo file with the id of the first
// one (shadowed), a repo file with no id and a second repo file with the id of
// the invalid one.
func scan() *catalog.Result {
	a := &catalog.File{Layer: catalog.LayerRepo, Name: "a.yaml", Display: "jitso/builds/a.yaml", ID: "a"}
	broken := &catalog.File{Layer: catalog.LayerRepo, Name: "broken.yaml", Display: "jitso/builds/broken.yaml", ID: "broken",
		Problems: []string{`model "x" is not in the allowlist`, `effort "y" is not in the allowlist`}}
	userA := &catalog.File{Layer: catalog.LayerRepo, Name: "z-a.yaml", Display: "jitso/builds/z-a.yaml", ID: "a", ShadowedBy: a}
	noID := &catalog.File{Layer: catalog.LayerRepo, Name: "z-noid.yaml", Display: "jitso/builds/z-noid.yaml", Problems: []string{"id must be kebab-case"}}
	userBroken := &catalog.File{Layer: catalog.LayerRepo, Name: "z-broken.yaml", Display: "jitso/builds/z-broken.yaml", ID: "broken", ShadowedBy: broken,
		Problems: []string{"status must be one of candidate, approved, retired"}}
	return &catalog.Result{Files: []*catalog.File{a, broken, userA, noID, userBroken}}
}

func noLaunchProblems(*catalog.File) []string { return nil }

func TestValidateReport(t *testing.T) {
	launchOnA := func(f *catalog.File) []string {
		if f.Name == "a.yaml" {
			return []string{"harness \"other\" has no adapter"}
		}
		return nil
	}
	tests := []struct {
		name    string
		id      string
		extra   func(*catalog.File) []string
		want    []string
		invalid int
	}{
		{"every file, in order", "", noLaunchProblems, []string{
			"ok a repo jitso/builds/a.yaml",
			"invalid broken repo jitso/builds/broken.yaml",
			`  model "x" is not in the allowlist`,
			`  effort "y" is not in the allowlist`,
			"ok a repo jitso/builds/z-a.yaml (shadowed by repo jitso/builds/a.yaml)",
			"invalid - repo jitso/builds/z-noid.yaml",
			"  id must be kebab-case",
			"invalid broken repo jitso/builds/z-broken.yaml (shadowed by repo jitso/builds/broken.yaml)",
			"  status must be one of candidate, approved, retired",
			"checked 5 builds: 2 ok, 3 invalid",
		}, 3},
		{"launch problems follow the file's own", "", launchOnA, []string{
			"invalid a repo jitso/builds/a.yaml",
			"  harness \"other\" has no adapter",
			"invalid broken repo jitso/builds/broken.yaml",
			`  model "x" is not in the allowlist`,
			`  effort "y" is not in the allowlist`,
			"ok a repo jitso/builds/z-a.yaml (shadowed by repo jitso/builds/a.yaml)",
			"invalid - repo jitso/builds/z-noid.yaml",
			"  id must be kebab-case",
			"invalid broken repo jitso/builds/z-broken.yaml (shadowed by repo jitso/builds/broken.yaml)",
			"  status must be one of candidate, approved, retired",
			"checked 5 builds: 1 ok, 4 invalid",
		}, 4},
		{"one id: the winner and the files it shadows, the winner counted", "a", noLaunchProblems, []string{
			"ok a repo jitso/builds/a.yaml",
			"ok a repo jitso/builds/z-a.yaml (shadowed by repo jitso/builds/a.yaml)",
			"checked 1 builds: 1 ok, 0 invalid",
		}, 0},
		{"one id: an invalid winner, and an invalid shadowed file is not counted", "broken", noLaunchProblems, []string{
			"invalid broken repo jitso/builds/broken.yaml",
			`  model "x" is not in the allowlist`,
			`  effort "y" is not in the allowlist`,
			"invalid broken repo jitso/builds/z-broken.yaml (shadowed by repo jitso/builds/broken.yaml)",
			"  status must be one of candidate, approved, retired",
			"checked 1 builds: 0 ok, 1 invalid",
		}, 1},
		{"one id: a launch problem of the winner", "a", launchOnA, []string{
			"invalid a repo jitso/builds/a.yaml",
			"  harness \"other\" has no adapter",
			"ok a repo jitso/builds/z-a.yaml (shadowed by repo jitso/builds/a.yaml)",
			"checked 1 builds: 0 ok, 1 invalid",
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, invalid, err := validateReport(scan(), tt.id, tt.extra)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if invalid != tt.invalid {
				t.Errorf("invalid = %d, want %d", invalid, tt.invalid)
			}
		})
	}
}

func TestValidateReportEmptyAndUnknown(t *testing.T) {
	lines, invalid, err := validateReport(&catalog.Result{}, "", noLaunchProblems)
	if err != nil || invalid != 0 || !reflect.DeepEqual(lines, []string{"checked 0 builds: 0 ok, 0 invalid"}) {
		t.Errorf("empty scan: %q, %d, %v", lines, invalid, err)
	}
	_, _, err = validateReport(scan(), "nope", noLaunchProblems)
	if err == nil || err.Error() != `JITSO REJECTED: unknown build "nope"` || fail.Code(err) != fail.CodeRejected {
		t.Errorf("unknown id: %v (code %d)", err, fail.Code(err))
	}
}

func TestBuildValidateArguments(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--all"}, `JITSO REJECTED: unknown argument "--all"`},
		{[]string{"--json"}, `JITSO REJECTED: unknown argument "--json"`},
		{[]string{"-h"}, `JITSO REJECTED: unknown argument "-h"`},
		{[]string{"a", "b"}, `JITSO REJECTED: unknown argument "b"`},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			code, stdout, stderr := run(t, append([]string{"build", "validate"}, tt.args...)...)
			if code != 2 || stdout != "" || strings.TrimSpace(stderr) != tt.want {
				t.Errorf("code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
}

func TestBuildUsageListsValidate(t *testing.T) {
	if u := BuildUsage(); !strings.Contains(u, "  validate [<id>]                check every build, or one, against this project's allowlists") {
		t.Errorf("usage lacks the validate line:\n%s", u)
	}
	list, validate := strings.Index(BuildUsage(), "  list"), strings.Index(BuildUsage(), "  validate")
	if list < 0 || validate < list {
		t.Errorf("validate must come after list in the usage:\n%s", BuildUsage())
	}
}
