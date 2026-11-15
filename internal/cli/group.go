package cli

import "github.com/upex-galaxy/jitso/internal/fail"

// GroupSub is one second word of a Group.
type GroupSub struct {
	// Name is the word that selects the subcommand.
	Name string
	// Run executes it with the words after its name.
	Run func(ctx *Context, args []string) error
}

// Group is a command whose first argument picks a subcommand (`jitso install
// skill`, `jitso setup hook`), answering like `jitso build` does:
//
//	<group>                     "jitso <group>: <Missing>", a blank line, the usage, exit 2
//	<group> --help | -h         the usage on stdout, exit 0
//	<group> <unknown>           `jitso <group>: unknown subcommand "<x>"`, a blank line, the usage, exit 2
//	<group> <subcommand> ...    that subcommand
//
// The group is a value: it registers nothing. A command file builds one and
// hands Command to Register.
type Group struct {
	// Name is the first word of the command line.
	Name string
	// Usage is the complete usage text, printed as is.
	Usage string
	// Missing is the text after "jitso <group>: " when no subcommand is given;
	// "" means "missing subcommand".
	Missing string
	// Subs are the subcommands.
	Subs []GroupSub
}

// Run is the command body of the group.
func (g Group) Run(ctx *Context, args []string) error {
	if len(args) == 0 {
		missing := g.Missing
		if missing == "" {
			missing = "missing subcommand"
		}
		return fail.Usage("jitso " + g.Name + ": " + missing + "\n\n" + g.Usage)
	}
	first := args[0]
	if first == "--help" || first == "-h" {
		ctx.Println(g.Usage)
		return nil
	}
	for _, s := range g.Subs {
		if s.Name == first {
			return s.Run(ctx, args[1:])
		}
	}
	return fail.Usage("jitso " + g.Name + ": unknown subcommand \"" + first + "\"\n\n" + g.Usage)
}

// Command is the group as a table entry, ready for Register.
func (g Group) Command(order int, summary string) Command {
	return Command{Name: g.Name, Order: order, Summary: summary, Run: g.Run}
}
