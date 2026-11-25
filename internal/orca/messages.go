package orca

import (
	"encoding/json"
	"strconv"
)

// This file holds the calls jitso graph needs besides the terminal list: the
// messages sessions sent each other, the addresses those messages use and the
// switch to a terminal tab.

// Message is who sent a message of the orchestration inbox, to whom, of what
// kind and when. It has no field for the subject, the body or the payload:
// jitso never keeps what a session said.
type Message struct {
	// From and To are addresses: a terminal handle, run:<run id> (the
	// coordinator of the run) or dispatch:<dispatch id> (the worker of the
	// dispatch).
	From, To string
	// Type is Orca's message type, for example status or heartbeat.
	Type string
	// RunID is the run the message belongs to, "" when Orca names none.
	RunID string
	// CreatedAt is Orca's timestamp text.
	CreatedAt string
}

// Messages runs `orchestration inbox --limit <limit> --full` (every mailbox of
// this Orca, newest first; the concise form carries no timestamp) and decodes
// only the fields of Message, so the content of a message never enters a value
// of this program. The errors of this one call carry the error code or the exit
// status and nothing Orca wrote, because what it wrote may hold content.
func (c *Client) Messages(limit int) ([]Message, error) {
	out := c.run([]string{"orchestration", "inbox", "--limit", strconv.Itoa(limit), "--full"})
	if out.Status == StatusTimeout {
		return nil, timedOut("orchestration inbox", out)
	}
	var reply struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Result struct {
			Messages []struct {
				From      string `json:"from_handle"`
				To        string `json:"to_handle"`
				Type      string `json:"type"`
				RunID     string `json:"run_id"`
				CreatedAt string `json:"created_at"`
			} `json:"messages"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.Stdout), &reply); err != nil {
		return nil, &Error{Message: "orca orchestration inbox gave no readable JSON (exit " + status(out.Status) + ")"}
	}
	if !reply.OK {
		why := reply.Error.Code
		if why == "" {
			why = "exit " + status(out.Status)
		}
		return nil, &Error{Code: reply.Error.Code, Message: "orca orchestration inbox failed (" + why + ")"}
	}
	if out.Status != 0 && out.Status != -1 {
		return nil, &Error{Message: "orca orchestration inbox exited " + strconv.Itoa(out.Status)}
	}
	messages := make([]Message, 0, len(reply.Result.Messages))
	for _, m := range reply.Result.Messages {
		messages = append(messages, Message{From: m.From, To: m.To, Type: m.Type, RunID: m.RunID, CreatedAt: m.CreatedAt})
	}
	return messages, nil
}

// workerPage is the page size of `worker-list` (Orca's maximum), and maxPages
// stops a cursor that never ends.
const (
	workerPage = 100
	maxPages   = 100
)

// pages runs a list verb and follows its cursor: each page goes to read, which
// returns the cursor of the next one, "" at the end.
func (c *Client) pages(args []string, read func(result map[string]any) (next string)) error {
	cursor := ""
	for range maxPages {
		call := args
		if cursor != "" {
			call = append(append([]string(nil), args...), "--cursor", cursor)
		}
		res, err := c.Call(call)
		if err != nil {
			return err
		}
		if cursor = read(res); cursor == "" {
			break
		}
	}
	return nil
}

// RunCoordinators runs `orchestration run-list --full` and returns the
// coordinator terminal of each run that has one, by run id.
func (c *Client) RunCoordinators() (map[string]string, error) {
	coordinators := map[string]string{}
	err := c.pages([]string{"orchestration", "run-list", "--full"}, func(res map[string]any) string {
		runs, _ := res["runs"].([]any)
		for _, item := range runs {
			run, _ := item.(map[string]any)
			id, _ := run["id"].(string)
			if handle, _ := run["coordinator_handle"].(string); id != "" && handle != "" {
				coordinators[id] = handle
			}
		}
		next, _ := res["nextCursor"].(string)
		return next
	})
	return coordinators, err
}

// RunWorkers runs `orchestration worker-list --run <run> --full` and returns
// the terminal of each dispatch of the run that has one, by dispatch id. The
// run is always named: without it Orca lists the run of the calling terminal.
func (c *Client) RunWorkers(run string) (map[string]string, error) {
	terminals := map[string]string{}
	args := []string{"orchestration", "worker-list", "--run", run, "--limit", strconv.Itoa(workerPage), "--full"}
	err := c.pages(args, func(res map[string]any) string {
		workers, _ := res["workers"].([]any)
		for _, item := range workers {
			w, _ := item.(map[string]any)
			id, _ := w["dispatchId"].(string)
			handle, _ := w["agentTerminalHandle"].(string)
			if handle == "" {
				resource, _ := w["resource"].(map[string]any)
				handle, _ = resource["terminalHandle"].(string)
			}
			if id != "" && handle != "" {
				terminals[id] = handle
			}
		}
		page, _ := res["page"].(map[string]any)
		next, _ := page["nextCursor"].(string)
		return next
	})
	return terminals, err
}

// SwitchTerminal runs `terminal switch --terminal <handle>`: it changes the
// terminal the person is looking at, so only their own action calls it.
func (c *Client) SwitchTerminal(handle string) error {
	_, err := c.Call([]string{"terminal", "switch", "--terminal", handle})
	return err
}
