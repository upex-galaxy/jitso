package jsonx

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// Lookup follows a dot path (`a.b.0.c`) into v. A numeric segment indexes a list; on an
// object it is a key. The empty path is v itself.
func Lookup(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	cur := v
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// Equal compares two JSON values, numbers by value (`0.5` equals `0.50`).
func Equal(a, b any) bool {
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func normalize(v any) any {
	switch n := v.(type) {
	case json.Number:
		f, err := strconv.ParseFloat(n.String(), 64)
		if err != nil {
			return n.String()
		}
		return f
	case int:
		return float64(n)
	case map[string]any:
		out := make(map[string]any, len(n))
		for k, item := range n {
			out[k] = normalize(item)
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, item := range n {
			out[i] = normalize(item)
		}
		return out
	}
	return v
}

// StringForm is what a regex is tested against: a string as is, anything else as compact JSON.
func StringForm(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// Compact renders v as compact JSON for messages.
func Compact(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "<unprintable>"
	}
	return string(data)
}
