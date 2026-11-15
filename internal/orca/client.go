package orca

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Error is a failed orca call. Code is Orca's own error code, "" when it gave none.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Error codes the callers test for.
const (
	// CodeHandleStale is the code of a call on a terminal that no longer exists.
	CodeHandleStale = "terminal_handle_stale"
)

// Client runs orca verbs through a Runner.
type Client struct {
	run Runner
}

// NewClient is a client over run.
func NewClient(run Runner) *Client { return &Client{run: run} }

// Call runs one verb and returns the result object of the envelope
// ({"ok": true, "result": {...}}). A reply that is not JSON, ok not true, or a
// non-zero exit is an *Error, with the oracle's messages:
// `orca <verb> gave no JSON (exit <n>): <stderr>`, `orca <verb> failed:
// <message>`, `orca <verb> exited <n>`.
func (c *Client) Call(args []string) (map[string]any, error) {
	out := c.run(args)
	verb := strings.Join(args[:min(2, len(args))], " ")
	var parsed any
	if err := json.Unmarshal([]byte(out.Stdout), &parsed); err != nil {
		return nil, &Error{Message: "orca " + verb + " gave no JSON (exit " + status(out.Status) + "): " + clip(strings.TrimSpace(out.Stderr), 200)}
	}
	envelope, _ := parsed.(map[string]any)
	if ok, _ := envelope["ok"].(bool); !ok {
		body, _ := envelope["error"].(map[string]any)
		code, _ := body["code"].(string)
		msg := "unknown error"
		if m, present := body["message"]; present && m != nil {
			msg = jsconv.String(m)
		} else if c, present := body["code"]; present && c != nil {
			msg = jsconv.String(c)
		}
		return nil, &Error{Code: code, Message: "orca " + verb + " failed: " + msg}
	}
	if out.Status != 0 && out.Status != -1 {
		return nil, &Error{Message: "orca " + verb + " exited " + strconv.Itoa(out.Status)}
	}
	result, _ := envelope["result"].(map[string]any)
	if result == nil {
		result = map[string]any{}
	}
	return result, nil
}

func status(s int) string {
	if s == -1 {
		return "null"
	}
	return strconv.Itoa(s)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Terminal is a terminal orca created.
type Terminal struct {
	Handle string
	// WorktreePath is the folder the terminal runs in, from Orca's own answer
	// (the path part of worktreeId), "" when Orca reported none.
	WorktreePath string
}

// WorktreePath is the path of a worktreeId (`<uuid>::<absolute path>`), and
// false when the id has no "::".
func WorktreePath(worktreeID string) (string, bool) {
	_, path, ok := strings.Cut(worktreeID, "::")
	return path, ok
}

// CreateTerminal runs `terminal create [--worktree <w>] --title <t> --command <c>`.
// An empty worktree leaves the flag out.
func (c *Client) CreateTerminal(worktree, title, command string) (Terminal, error) {
	args := []string{"terminal", "create"}
	if worktree != "" {
		args = append(args, "--worktree", worktree)
	}
	args = append(args, "--title", title, "--command", command)
	res, err := c.Call(args)
	if err != nil {
		return Terminal{}, err
	}
	term, _ := res["terminal"].(map[string]any)
	handle, ok := term["handle"].(string)
	if !ok {
		return Terminal{}, &Error{Message: "orca terminal create returned no handle"}
	}
	t := Terminal{Handle: handle}
	if id, ok := term["worktreeId"].(string); ok {
		t.WorktreePath, _ = WorktreePath(id)
	}
	return t, nil
}

// WaitIdle runs `terminal wait --for tui-idle` and reports whether the
// terminal got there. A failed call (a timeout) is false, never an error.
func (c *Client) WaitIdle(handle string, timeoutMS int) bool {
	_, err := c.Call([]string{"terminal", "wait", "--terminal", handle, "--for", "tui-idle", "--timeout-ms", strconv.Itoa(timeoutMS)})
	return err == nil
}

// ReadScreen runs `terminal read --screen` and returns the rows joined with
// newlines (each row through String()).
func (c *Client) ReadScreen(handle string) (string, error) {
	res, err := c.Call([]string{"terminal", "read", "--terminal", handle, "--screen"})
	if err != nil {
		return "", err
	}
	term, _ := res["terminal"].(map[string]any)
	tail, ok := term["tail"].([]any)
	if !ok {
		return "", nil
	}
	rows := make([]string, len(tail))
	for i, row := range tail {
		rows[i] = jsconv.String(row)
	}
	return strings.Join(rows, "\n"), nil
}

// SendText sends raw text to the terminal, no Enter appended.
func (c *Client) SendText(handle, text string) error {
	_, err := c.Call([]string{"terminal", "send", "--terminal", handle, "--text", text})
	return err
}

// CloseTerminal closes one terminal (never the worktree-wide close).
func (c *Client) CloseTerminal(handle string) error {
	_, err := c.Call([]string{"terminal", "close", "--terminal", handle})
	return err
}

// ListHandles runs `terminal list` and returns the handles it reports.
func (c *Client) ListHandles() ([]string, error) {
	res, err := c.Call([]string{"terminal", "list"})
	if err != nil {
		return nil, err
	}
	list, _ := res["terminals"].([]any)
	handles := []string{}
	for _, t := range list {
		m, _ := t.(map[string]any)
		if h, ok := m["handle"].(string); ok {
			handles = append(handles, h)
		}
	}
	return handles, nil
}
