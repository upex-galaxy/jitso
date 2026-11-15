// Package jsonx holds the small helpers the runner uses to read a decoded JSON tree
// (map[string]any, []any, string, json.Number, bool, nil) strictly: every unknown key and
// every wrong type is an error that names where it happened.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Decode parses one JSON value, keeping numbers as json.Number. Trailing data is an error.
func Decode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the JSON value")
	}
	return v, nil
}

// Object returns v as an object.
func Object(v any, what string) (map[string]any, error) {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: want an object, got %s", what, Kind(v))
	}
	return obj, nil
}

// Keys fails on the first key of obj that is not in allowed.
func Keys(obj map[string]any, what string, allowed ...string) error {
	names := make([]string, 0, len(obj))
	for k := range obj {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		known := false
		for _, a := range allowed {
			if k == a {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("%s: unknown key %q (allowed: %s)", what, k, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// String returns v as a string.
func String(v any, what string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: want a string, got %s", what, Kind(v))
	}
	return s, nil
}

// Bool returns v as a boolean.
func Bool(v any, what string) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s: want true or false, got %s", what, Kind(v))
	}
	return b, nil
}

// Int returns v as an integer.
func Int(v any, what string) (int, error) {
	switch n := v.(type) {
	case json.Number:
		i, err := strconv.Atoi(n.String())
		if err != nil {
			return 0, fmt.Errorf("%s: want an integer, got %s", what, n)
		}
		return i, nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("%s: want an integer, got %v", what, n)
		}
		return int(n), nil
	}
	return 0, fmt.Errorf("%s: want an integer, got %s", what, Kind(v))
}

// Strings returns v as a list of strings.
func Strings(v any, what string) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: want a list of strings, got %s", what, Kind(v))
	}
	out := make([]string, len(list))
	for i, item := range list {
		s, err := String(item, fmt.Sprintf("%s[%d]", what, i))
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// List returns v as a list.
func List(v any, what string) ([]any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: want a list, got %s", what, Kind(v))
	}
	return list, nil
}

// StringMap returns v as an object whose values are all strings.
func StringMap(v any, what string) (map[string]string, error) {
	obj, err := Object(v, what)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(obj))
	for k, item := range obj {
		s, err := String(item, what+"."+k)
		if err != nil {
			return nil, err
		}
		out[k] = s
	}
	return out, nil
}

// SortedKeys returns the keys of obj in byte order, so iteration is deterministic.
func SortedKeys[V any](obj map[string]V) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Kind names the JSON type of v for error messages.
func Kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "an object"
	case []any:
		return "a list"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case json.Number, float64:
		return "a number"
	}
	return fmt.Sprintf("%T", v)
}
