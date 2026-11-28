package launch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/harness"
)

// ConnectorsTimeout is how long `claude mcp list` may take. It health-checks
// every server the owner has, so it is slow; DECIDED: one minute, after which
// the launch fails instead of waiting on a server that does not answer.
const ConnectorsTimeout = 60 * time.Second

// ConnectorPrefix starts the name `claude mcp list` gives a claude.ai
// connector.
const ConnectorPrefix = "claude.ai "

// cloudItems are the connector names of the cloud:<name> items among mcp, in
// order, each once.
func cloudItems(mcp []string) []string {
	var out []string
	for _, item := range mcp {
		if name, ok := catalog.CloudName(item); ok && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// serverNames are the names the session sees for the servers of its own mcp
// file: a user:<name> server under its plain name, a connector not at all.
func serverNames(mcp []string) []string {
	var out []string
	for _, item := range mcp {
		if _, ok := catalog.CloudName(item); ok {
			continue
		}
		if name, ok := catalog.UserName(item); ok {
			item = name
		}
		out = append(out, item)
	}
	return out
}

// listedConnector is one claude.ai connector of `claude mcp list`.
type listedConnector struct {
	// name is the connector's name without the "claude.ai " prefix.
	name string
	// url is the first word after the colon, "" when it is not an http(s) URL.
	url string
}

// ParseConnectors reads the output of `claude mcp list`: a line that is
// "<name>: <target> ..." is a connector when its name starts with "claude.ai ",
// in any case. The name is the text before the first ": ", the target the next
// word. Any other line, and the command or arguments of any server, are never
// kept.
func ParseConnectors(out string) []listedConnector {
	var found []listedConnector
	for _, line := range strings.Split(out, "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok || len(name) < len(ConnectorPrefix) || !strings.EqualFold(name[:len(ConnectorPrefix)], ConnectorPrefix) {
			continue
		}
		c := listedConnector{name: strings.TrimSpace(name[len(ConnectorPrefix):])}
		if fields := strings.Fields(rest); len(fields) > 0 && (strings.HasPrefix(fields[0], "https://") || strings.HasPrefix(fields[0], "http://")) {
			c.url = fields[0]
		}
		found = append(found, c)
	}
	return found
}

// resolveConnectors finds, for each name, the claude.ai connector of that name
// (case-insensitive) in the output of list. A name no connector has is a
// rejection that lists the connectors there are; a list that fails is an
// environment error with a one-line reason.
func resolveConnectors(names []string, list func() (string, error)) ([]harness.CloudConnector, error) {
	out, err := list()
	if err != nil {
		return nil, fail.Envf("could not list the claude.ai connectors: %s", err.Error())
	}
	listed := ParseConnectors(out)
	var resolved []harness.CloudConnector
	for _, name := range names {
		var hit *listedConnector
		for i := range listed {
			if strings.EqualFold(listed[i].name, name) {
				hit = &listed[i]
				break
			}
		}
		if hit == nil {
			var available []string
			for _, c := range listed {
				if !slices.Contains(available, c.name) {
					available = append(available, c.name)
				}
			}
			list := "none"
			if len(available) > 0 {
				list = strings.Join(available, ", ")
			}
			return nil, fail.RejectedEnvf(`no claude.ai connector named "%s" (available: %s)`, name, list)
		}
		if hit.url == "" {
			return nil, fail.Envf(`the claude.ai connector "%s" has no URL in the claude mcp list output`, hit.name)
		}
		resolved = append(resolved, harness.CloudConnector{Name: name, URL: hit.url})
	}
	return resolved, nil
}

// runMCPList runs `<binary> mcp list` found on PATH, the way run.sh finds the
// session's own program, and returns its stdout. Its stderr and any server
// command it shows are never passed on: a failure is a short reason.
func runMCPList(binary string, timeout time.Duration) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", errors.New(binary + " is not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "mcp", "list")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", errors.New(binary + " mcp list did not answer in " + timeout.String())
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", errors.New(binary + " mcp list failed with exit status " + strconv.Itoa(exit.ExitCode()))
		}
		return "", errors.New(binary + " mcp list could not run")
	}
	return stdout.String(), nil
}
