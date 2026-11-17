// Package jsonx is the one JSON encoder setup every jitso writer uses: no HTML
// escaping (reasons and briefs carry "<id>" and "&"), compact lines for the
// registry, an indented form for files such as mcp.json, UTC timestamps with
// milliseconds, and an ordered object type for maps whose key order is
// kept (insertion order) where a Go map would sort the keys.
//
// Floats use encoding/json's shortest round-trip form, which matches
// JSON.stringify for finite numbers. Two known differences from JSON.stringify:
// U+2028 and U+2029 are written as \u2028 and \u2029, and invalid UTF-8 becomes
// U+FFFD.
package jsonx

import (
	"bytes"
	"encoding/json"
	"time"
)

// TimeLayout is the form of JavaScript's Date.prototype.toISOString.
const TimeLayout = "2006-01-02T15:04:05.000Z"

// Time formats t like Date.prototype.toISOString: UTC with milliseconds.
func Time(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// Marshal encodes v compactly without HTML escaping and without a trailing
// newline (JSON.stringify(v)).
func Marshal(v any) ([]byte, error) {
	return encode(v, "")
}

// MarshalIndent encodes v with the given indent, without HTML escaping and
// without a trailing newline (JSON.stringify(v, null, indent)).
func MarshalIndent(v any, indent string) ([]byte, error) {
	return encode(v, indent)
}

func encode(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Field is one key and value of an Object.
type Field struct {
	Key   string
	Value any
}

// Object is a JSON object whose keys are written in slice order. A later field
// with the same key as an earlier one is written too (callers keep keys unique).
type Object []Field

// MarshalJSON writes the fields in order.
func (o Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := Marshal(f.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := Marshal(f.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
