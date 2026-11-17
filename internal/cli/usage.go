package cli

import (
	"fmt"
	"strings"
)

// Usage is the usage text: one line per command in Order, then --help and
// the exit codes.
func Usage() string {
	var b strings.Builder
	b.WriteString("Usage: jitso <command> [options]\n")
	b.WriteString("\n")
	for _, c := range Commands() {
		fmt.Fprintf(&b, "  %-8s %s\n", c.Name, c.Summary)
	}
	b.WriteString("\n  --help   print this usage and exit 0\n\n")
	b.WriteString("Exit codes: 0 done, 2 usage or rejected by validation, 3 Orca or harness error.")
	return b.String()
}
