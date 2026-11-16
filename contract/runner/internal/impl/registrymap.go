package impl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"jitso-contract-runner/internal/sandbox"
)

// The registry mapping of the ts adapter (SPEC 9, defect N-31). The oracle reads and writes
// `v: 1` registry lines whose build keys are `preset` and `preset_version`; the cases and the
// Go port use `v: 2` with `build` and `build_version`. Removed together with the oracle.

// registryRel is the registry log, relative to the project (the file the registry matcher
// reads, expect.RegistryPath).
var registryRel = filepath.Join(".jitso", "launches.jsonl")

var (
	downKeys = map[string]string{"build": "preset", "build_version": "preset_version"}
	upKeys   = map[string]string{"preset": "build", "preset_version": "build_version"}
)

// DownText turns every line of a registry file that is one JSON object whose top-level `v`
// is the number 2 into the oracle's `v: 1` form. Every other line, a blank one included,
// passes unchanged.
func DownText(data []byte) []byte { return mapText(data, 2, 1, downKeys) }

// UpText is the reverse of DownText: `v: 1` lines become `v: 2` lines.
func UpText(data []byte) []byte { return mapText(data, 1, 2, upKeys) }

// DownLine and UpLine are DownText and UpText for one line.
func DownLine(line []byte) []byte { return rewrite(line, 2, 1, downKeys) }
func UpLine(line []byte) []byte   { return rewrite(line, 1, 2, upKeys) }

func mapText(data []byte, from, to int, keys map[string]string) []byte {
	parts := bytes.Split(data, []byte("\n"))
	for i, p := range parts {
		parts[i] = rewrite(p, from, to, keys)
	}
	return bytes.Join(parts, []byte("\n"))
}

type span struct{ start, end int }

// rewrite changes one JSON object line: every top-level key token named in keys is renamed,
// and every top-level `v` value token equal to from becomes to, but only when the last
// top-level `v` is a number equal to from (the value JSON.parse and Go would both take).
// Only those tokens change, never a re-encode, so spacing, key order, escapes and number
// spelling stay. A line that is not exactly one JSON object is returned unchanged.
func rewrite(line []byte, from, to int, keys map[string]string) []byte {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	// tokSpan is the raw bytes of the token read since offset before: the decoder returns no
	// token for the whitespace, comma and colon bytes between tokens, so they are skipped.
	tokSpan := func(before int) span {
		end := int(dec.InputOffset())
		start := before
		for start < end && bytes.IndexByte([]byte(" \t\r\n,:"), line[start]) >= 0 {
			start++
		}
		return span{start, end}
	}
	if d, ok := token(dec).(json.Delim); !ok || d != '{' {
		return line
	}
	var keySpans []span
	var keyNames []string
	var vSpans []span
	lastMatches := false
	for dec.More() {
		before := int(dec.InputOffset())
		k, err := dec.Token()
		name, isString := k.(string)
		if err != nil || !isString {
			return line
		}
		if _, hit := keys[name]; hit {
			keySpans = append(keySpans, tokSpan(before))
			keyNames = append(keyNames, name)
		}
		before = int(dec.InputOffset())
		v, err := dec.Token()
		if err != nil {
			return line
		}
		if d, isDelim := v.(json.Delim); isDelim && (d == '{' || d == '[') {
			// A nested value: skip to its end. A nested `v` or `build` is not top-level.
			for depth := 1; depth > 0; {
				x, err := dec.Token()
				if err != nil {
					return line
				}
				if dd, ok := x.(json.Delim); ok {
					if dd == '{' || dd == '[' {
						depth++
					} else {
						depth--
					}
				}
			}
			if name == "v" {
				lastMatches = false
			}
			continue
		}
		if name == "v" {
			lastMatches = false
			if n, ok := v.(json.Number); ok {
				if f, err := n.Float64(); err == nil && f == float64(from) {
					lastMatches = true
					vSpans = append(vSpans, tokSpan(before))
				}
			}
		}
	}
	if d, ok := token(dec).(json.Delim); !ok || d != '}' {
		return line
	}
	if _, err := dec.Token(); err != io.EOF {
		return line
	}
	if !lastMatches {
		return line
	}
	type edit struct {
		s    span
		text string
	}
	var edits []edit
	for i, s := range keySpans {
		edits = append(edits, edit{s, `"` + keys[keyNames[i]] + `"`})
	}
	for _, s := range vSpans {
		edits = append(edits, edit{s, fmt.Sprint(to)})
	}
	// Spans are disjoint and found left to right per kind; apply the edits right to left so
	// the offsets of the ones still to apply stay valid.
	for i := range edits {
		for j := i + 1; j < len(edits); j++ {
			if edits[j].s.start > edits[i].s.start {
				edits[i], edits[j] = edits[j], edits[i]
			}
		}
	}
	out := append([]byte(nil), line...)
	for _, e := range edits {
		tail := append([]byte(e.text), out[e.s.end:]...)
		out = append(out[:e.s.start], tail...)
	}
	return out
}

// token reads the next token and returns nil on an error.
func token(dec *json.Decoder) json.Token {
	t, err := dec.Token()
	if err != nil {
		return nil
	}
	return t
}

// AroundStep maps the registry file around one oracle step (SPEC 9). Before the step every
// `v: 2` line of the case becomes a `v: 1` line. After it the file gets the case's own bytes
// back and every line the step appended becomes a `v: 2` line. The state lives in the
// closure, never in the TS value: sandboxes run in parallel.
func (t *TS) AroundStep(sb *sandbox.Sandbox) (func() error, error) {
	path := filepath.Join(sb.Project, registryRel)
	orig, err := os.ReadFile(path)
	absent := errors.Is(err, fs.ErrNotExist)
	if err != nil && !absent {
		return nil, err
	}
	down := DownText(orig)
	if !absent && !bytes.Equal(down, orig) {
		if err := os.WriteFile(path, down, 0o644); err != nil {
			return nil, err
		}
	}
	return func() error {
		now, err := os.ReadFile(path)
		nowAbsent := errors.Is(err, fs.ErrNotExist)
		if err != nil && !nowAbsent {
			return err
		}
		if nowAbsent {
			if absent {
				return nil
			}
			return fmt.Errorf("the oracle removed .jitso/launches.jsonl")
		}
		appended := now
		if !absent {
			if !bytes.HasPrefix(now, down) {
				return fmt.Errorf("the oracle rewrote .jitso/launches.jsonl")
			}
			appended = now[len(down):]
		}
		final := append(append([]byte(nil), orig...), UpText(appended)...)
		if bytes.Equal(final, now) {
			return nil
		}
		return os.WriteFile(path, final, 0o644)
	}, nil
}
