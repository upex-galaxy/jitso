package catalog

import "github.com/upex-galaxy/jitso/internal/jsconv"

// Map is a decoded YAML mapping with its keys in document order. A JavaScript
// object built from the same YAML has string keys, so every key is kept as its
// String() text.
type Map struct {
	keys []string
	vals map[string]any
}

// JSObject marks Map as an object for jsconv.String ("[object Object]").
func (*Map) JSObject() {}

func (m *Map) set(key string, val any) {
	if m.vals == nil {
		m.vals = map[string]any{}
	}
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

// Get returns the value of key and whether the key is present.
func (m *Map) Get(key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.vals[key]
	return v, ok
}

// Value is the value of key, or jsconv.Undefined when the key is missing.
func (m *Map) Value(key string) any {
	return jsconv.Of(m.Get(key))
}

// Keys are the keys in document order.
func (m *Map) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// Values are the values in key order (Object.values).
func (m *Map) Values() []any {
	if m == nil {
		return nil
	}
	out := make([]any, len(m.keys))
	for i, k := range m.keys {
		out[i] = m.vals[k]
	}
	return out
}

// keyText is the property name a JavaScript object gives a YAML key.
func keyText(key any) string {
	return jsconv.String(key)
}
