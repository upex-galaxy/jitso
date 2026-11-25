package graph

import (
	"strconv"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Usage is the usage line, in every rejection and for --help.
const Usage = "usage: jitso graph [--json] [--port <n>]"

// Options are the flags of the command.
type Options struct {
	// JSON prints one snapshot and exits instead of serving.
	JSON bool
	// Port is the port to serve on, 0 for any free one.
	Port int
}

// ParseArgs reads the flags: --json, or --port with a number from 0 to 65535,
// each at most once and never together. Anything else rejects the call (exit
// 2).
func ParseArgs(args []string) (Options, error) {
	var o Options
	port := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json" && !o.JSON:
			o.JSON = true
		case args[i] == "--port" && !port && i+1 < len(args):
			i++
			n, err := strconv.ParseUint(args[i], 10, 16)
			if err != nil {
				return Options{}, fail.Rejected(Usage)
			}
			o.Port, port = int(n), true
		default:
			return Options{}, fail.Rejected(Usage)
		}
	}
	if o.JSON && port {
		return Options{}, fail.Rejected(Usage)
	}
	return o, nil
}
