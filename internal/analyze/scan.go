package analyze

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"strconv"
)

// forEachLine calls fn for every non-empty line of r, split on "\n" only, the
// last line included even without a newline. It reads through a bufio.Reader,
// not a Scanner, because transcript lines can be far longer than a Scanner's
// token limit.
func forEachLine(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 {
			fn(line)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// row is the usage of one request: the family and the five token counts.
type row struct {
	family                                     string
	input, cacheRead, write5m, write1h, output float64
}

// toolCounts counts tool calls by name and remembers the first-seen order.
type toolCounts struct {
	order  []string
	counts map[string]int
}

func newToolCounts() *toolCounts { return &toolCounts{counts: map[string]int{}} }

func (t *toolCounts) add(name string) {
	if _, ok := t.counts[name]; !ok {
		t.order = append(t.order, name)
	}
	t.counts[name]++
}

// scanResult is what one transcript file contributes.
type scanResult struct {
	rows        []row
	secretReads int
}

// num is the oracle's num(): a finite JSON number, else 0.
func num(v any) float64 {
	f, ok := v.(float64)
	if !ok || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0
	}
	return f
}

// nonEmpty is the oracle's "an object with at least one key" for a decoded
// value; arrays count too (JavaScript's typeof says object).
func nonEmpty(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		return len(x) > 0
	case []any:
		return len(x) > 0
	}
	return false
}

// field reads a key of a decoded object; an array has no named keys.
func field(v any, key string) any {
	m, _ := v.(map[string]any)
	return m[key]
}

// dedupKey is the key a request is counted once under: message.id, else the
// line uuid. JavaScript's Set compares strings, numbers and booleans by value
// and objects by reference, so an object or array key is never "seen".
func dedupKey(id, uuid any, idPresent, uuidPresent bool) (key string, comparable bool) {
	v, present := id, idPresent
	if v == nil {
		v, present = uuid, uuidPresent
	}
	switch x := v.(type) {
	case string:
		return "s:" + x, true
	case float64:
		if x == 0 {
			x = 0 // -0 and 0 are the same Set member
		}
		return "n:" + strconv.FormatFloat(x, 'g', -1, 64), true
	case bool:
		return "b:" + strconv.FormatBool(x), true
	case nil:
		if present {
			return "z:", true
		}
		return "u:", true
	}
	return "", false
}

// scan reads one transcript stream. It collects the timestamps of every JSON
// object line into stamps and the tool names into tools; a transcript line is
// untrusted JSON, so every field is checked before use and none of its text
// leaves this function (only counts and tool names).
func scan(r io.Reader, tools *toolCounts, stamps *[]string) (scanResult, error) {
	seen := map[string]bool{}
	var res scanResult
	err := forEachLine(r, func(line []byte) {
		var parsed any
		if json.Unmarshal(line, &parsed) != nil {
			return
		}
		d, ok := parsed.(map[string]any)
		if !ok {
			return
		}
		if ts, ok := d["timestamp"].(string); ok && ts != "" {
			*stamps = append(*stamps, ts)
		}
		if d["type"] != "assistant" {
			return
		}
		message, _ := d["message"].(map[string]any)
		if content, ok := message["content"].([]any); ok {
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok || block["type"] != "tool_use" {
					continue
				}
				name, ok := block["name"].(string)
				if !ok {
					continue
				}
				tools.add(name)
				if touchesSecret(name, block["input"]) {
					res.secretReads++
				}
			}
		}
		usage := message["usage"]
		if !nonEmpty(usage) {
			return
		}
		id, idPresent := message["id"]
		uuid, uuidPresent := d["uuid"]
		key, comparable := dedupKey(id, uuid, idPresent, uuidPresent)
		if comparable && seen[key] {
			return
		}
		if comparable {
			seen[key] = true
		}
		cc := field(usage, "cache_creation")
		var w5, w1 float64
		if nonEmpty(cc) {
			w5, w1 = num(field(cc, "ephemeral_5m_input_tokens")), num(field(cc, "ephemeral_1h_input_tokens"))
		} else {
			w5 = num(field(usage, "cache_creation_input_tokens"))
		}
		res.rows = append(res.rows, row{
			family:    family(message["model"]),
			input:     num(field(usage, "input_tokens")),
			cacheRead: num(field(usage, "cache_read_input_tokens")),
			write5m:   w5,
			write1h:   w1,
			output:    num(field(usage, "output_tokens")),
		})
	})
	return res, err
}
