// Package usage sums the token counts of a Claude Code transcript: one JSONL
// stream in, the last usage of every message id summed out.
package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Totals is the sum of the token fields of a transcript and, once the
// subagent transcripts are added, of the whole session. Its JSON form is the
// tokens field of the end event.
type Totals struct {
	Input         int64 `json:"input"`
	CacheWrite    int64 `json:"cache_write"`
	CacheRead     int64 `json:"cache_read"`
	Output        int64 `json:"output"`
	Messages      int   `json:"messages"`
	SubagentFiles int   `json:"subagent_files"`
}

// Add folds another stream's totals into t. SubagentFiles is not summed: the
// caller counts the files it reads.
func (t *Totals) Add(o Totals) {
	t.Input += o.Input
	t.CacheWrite += o.CacheWrite
	t.CacheRead += o.CacheRead
	t.Output += o.Output
	t.Messages += o.Messages
}

type fields struct {
	Input      int64 `json:"input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	Output     int64 `json:"output_tokens"`
}

type line struct {
	Message struct {
		ID    string  `json:"id"`
		Usage *fields `json:"usage"`
	} `json:"message"`
}

// Sum reads one JSONL stream and keeps the last message.usage of each
// message.id: the transcript repeats a message once per content block and the
// later line holds the final counts. A line that is not JSON, or has no id or
// no usage, adds nothing. Lines have no length limit.
func Sum(r io.Reader) (Totals, error) {
	last := map[string]fields{}
	br := bufio.NewReader(r)
	for {
		raw, err := br.ReadBytes('\n')
		if raw = bytes.TrimSpace(raw); len(raw) > 0 {
			var l line
			if json.Unmarshal(raw, &l) == nil && l.Message.ID != "" && l.Message.Usage != nil {
				last[l.Message.ID] = *l.Message.Usage
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Totals{}, err
		}
	}
	t := Totals{Messages: len(last)}
	for _, f := range last {
		t.Input += f.Input
		t.CacheWrite += f.CacheWrite
		t.CacheRead += f.CacheRead
		t.Output += f.Output
	}
	return t, nil
}
