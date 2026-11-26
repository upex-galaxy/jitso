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
// non-zero exit is an *Error, with these messages:
// `orca <verb> gave no JSON (exit <n>): <stderr>`, `orca <verb> failed:
// <message>`, `orca <verb> exited <n>`.
func (c *Client) Call(args []string) (map[string]any, error) {
	out := c.run(args)
	verb := strings.Join(args[:min(2, len(args))], " ")
	if out.Status == StatusTimeout {
		return nil, timedOut(verb, out)
	}
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

// timedOut is the error of a call the runner gave up on.
func timedOut(verb string, out Result) error {
	return &Error{Code: CodeTimeout, Message: "orca " + verb + " " + out.Stderr}
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

// TrustBlock is the blockedReason Orca gives a terminal whose agent waits on
// its folder trust dialog.
const TrustBlock = "agent-trust-workspace"

// Wait is the answer of `terminal wait --for tui-idle`.
type Wait struct {
	// Idle is true when Orca reports the condition satisfied.
	Idle bool
	// Blocked is the blockedReason of a wait that is not satisfied, "" when
	// Orca named none.
	Blocked string
}

// WaitIdle runs `terminal wait --for tui-idle` and reads the answer. Orca
// exits 1 for a wait that is not satisfied, even with ok:true, so the JSON
// decides, never the exit status: `wait.satisfied` true is Idle, false with a
// `wait.blockedReason` is Blocked. Anything else (no JSON, ok not true, no
// `wait` object, a timeout) is the zero Wait.
func (c *Client) WaitIdle(handle string, timeoutMS int) Wait {
	out := c.run([]string{"terminal", "wait", "--terminal", handle, "--for", "tui-idle", "--timeout-ms", strconv.Itoa(timeoutMS)})
	var parsed any
	if err := json.Unmarshal([]byte(out.Stdout), &parsed); err != nil {
		return Wait{}
	}
	envelope, _ := parsed.(map[string]any)
	if ok, _ := envelope["ok"].(bool); !ok {
		return Wait{}
	}
	result, _ := envelope["result"].(map[string]any)
	wait, _ := result["wait"].(map[string]any)
	if satisfied, _ := wait["satisfied"].(bool); satisfied {
		return Wait{Idle: true}
	}
	reason, _ := wait["blockedReason"].(string)
	return Wait{Blocked: reason}
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

// ListLimit is the row limit ListHandles asks for. Orca's default page is not
// documented, so the call names a large one.
const ListLimit = 1000

// ListHandles runs `terminal list --limit <ListLimit>` and returns the handles
// it reports. A reply that is still truncated is an error: a partial list
// would make a live terminal look gone.
func (c *Client) ListHandles() ([]string, error) {
	terminals, err := c.ListTerminals()
	if err != nil {
		return nil, err
	}
	handles := make([]string, len(terminals))
	for i, t := range terminals {
		handles[i] = t.Handle
	}
	return handles, nil
}

// TerminalInfo is one row of `terminal list`: the fields of it jitso reads.
// Anything Orca left out is "".
type TerminalInfo struct {
	Handle string
	Title  string
	// Worktree is the folder of the terminal's worktree (worktreePath, else
	// the path part of worktreeId).
	Worktree string
	Branch   string
}

// ListTerminals runs `terminal list --limit <ListLimit>` and returns the
// terminals it reports that have a string handle, in Orca's order. A reply that is
// still truncated is an error, for the reason ListHandles gives.
func (c *Client) ListTerminals() ([]TerminalInfo, error) {
	res, err := c.Call([]string{"terminal", "list", "--limit", strconv.Itoa(ListLimit)})
	if err != nil {
		return nil, err
	}
	if truncated, _ := res["truncated"].(bool); truncated {
		return nil, &Error{Message: "orca terminal list is truncated: it did not return every terminal, so nothing was decided from it"}
	}
	list, _ := res["terminals"].([]any)
	terminals := []TerminalInfo{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		handle, ok := m["handle"].(string)
		if !ok {
			continue
		}
		t := TerminalInfo{Handle: handle}
		t.Title, _ = m["title"].(string)
		t.Branch, _ = m["branch"].(string)
		if path, ok := m["worktreePath"].(string); ok {
			t.Worktree = path
		} else if id, ok := m["worktreeId"].(string); ok {
			t.Worktree, _ = WorktreePath(id)
		}
		terminals = append(terminals, t)
	}
	return terminals, nil
}

// RepoPaths runs `repo list` and returns the folder of every repository Orca
// has registered (result.repos[].path), in Orca's order.
func (c *Client) RepoPaths() ([]string, error) {
	res, err := c.Call([]string{"repo", "list"})
	if err != nil {
		return nil, err
	}
	list, _ := res["repos"].([]any)
	paths := []string{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		if path, ok := m["path"].(string); ok && path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
