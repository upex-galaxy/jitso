package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// The build command is a group: `jitso build <subcommand>`. Each subcommand
// lives in its own cmd_build_<name>.go and registers itself into the group
// from an init function, the same way a command registers into the command
// table, so adding one never edits another subcommand's file.
//
//	build                  "jitso build: missing subcommand", a blank line, the usage, exit 2
//	build --help | -h      the usage on stdout, exit 0
//	build <unknown>        `jitso build: unknown subcommand "<x>"`, a blank line, the usage, exit 2
//	build <subcommand> ... that subcommand

// Subcommand is one second word of `jitso build`.
type Subcommand struct {
	// Name is the word that selects the subcommand.
	Name string
	// Usage is its line in the build usage, without the two leading spaces.
	Usage string
	// Order places the line in the build usage (ascending).
	Order int
	// Run executes the subcommand with the words after its name.
	Run func(ctx *Context, args []string) error
}

var subcommands = map[string]*Subcommand{}

// RegisterSubcommand adds a subcommand to `jitso build`. It panics on an empty
// or duplicate name, which is a programming error.
func RegisterSubcommand(s Subcommand) {
	if s.Name == "" || s.Run == nil {
		panic("cli: a subcommand needs a name and a Run function")
	}
	if _, dup := subcommands[s.Name]; dup {
		panic(fmt.Sprintf("cli: subcommand %q registered twice", s.Name))
	}
	subcommands[s.Name] = &s
}

// BuildUsage is the usage text of `jitso build`: one line per registered
// subcommand in Order.
func BuildUsage() string {
	list := make([]*Subcommand, 0, len(subcommands))
	for _, s := range subcommands {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].Name < list[j].Name
	})
	var b strings.Builder
	b.WriteString("Usage: jitso build <subcommand> [options]\n\n")
	for i, s := range list {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  " + s.Usage)
	}
	return b.String()
}

func init() {
	Register(Command{
		Name:    "build",
		Order:   60,
		Summary: "list, validate, create, draft, save and discard builds",
		Run:     runBuild,
	})
}

func runBuild(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fail.Usage("jitso build: missing subcommand\n\n" + BuildUsage())
	}
	first := args[0]
	if first == "--help" || first == "-h" {
		ctx.Println(BuildUsage())
		return nil
	}
	sub, ok := subcommands[first]
	if !ok {
		return fail.Usage(`jitso build: unknown subcommand "` + first + "\"\n\n" + BuildUsage())
	}
	return sub.Run(ctx, args[1:])
}
