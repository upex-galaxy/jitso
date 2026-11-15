// Package cli is the command layer: the command table, the usage text, the
// request rule (a first word with whitespace opens the dispatcher) and the one
// place an error becomes an exit code and a stderr text.
//
// Each command lives in its own cmd_<name>.go and registers itself from an
// init function, so parallel work on two commands never edits a shared file.
package cli

import (
	"fmt"
	"sort"
)

// Command is one first word of the command line.
type Command struct {
	// Name is the word that selects the command.
	Name string
	// Summary is its line in the usage text.
	Summary string
	// Order places it in the usage text (ascending). Usage order is fixed by
	// this field, never by file or init order.
	Order int
	// Run executes the command with the words after its name. It returns nil
	// for exit 0, or an error that fail.Code maps to the exit code.
	Run func(ctx *Context, args []string) error
}

var commands = map[string]*Command{}

// Register adds a command to the table. It panics on an empty or duplicate
// name, which is a programming error.
func Register(c Command) {
	if c.Name == "" || c.Run == nil {
		panic("cli: a command needs a name and a Run function")
	}
	if _, dup := commands[c.Name]; dup {
		panic(fmt.Sprintf("cli: command %q registered twice", c.Name))
	}
	commands[c.Name] = &c
}

// Lookup is the command with the given name.
func Lookup(name string) (*Command, bool) {
	c, ok := commands[name]
	return c, ok
}

// Commands are the registered commands in usage order.
func Commands() []*Command {
	list := make([]*Command, 0, len(commands))
	for _, c := range commands {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].Name < list[j].Name
	})
	return list
}
