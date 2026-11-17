package profile

import (
	"sort"
	"strconv"
)

// Fields are the --field name=value pairs of a request. A later value of a
// name replaces the earlier one. Keys come back in the order a JavaScript
// object gives its own keys (the fields are collected in one): names
// that are array indices first, ascending, then the others in the order they
// were first set. That order decides which bad field a rejection names.
type Fields struct {
	keys []string
	vals map[string]string
}

// Set records name=value.
func (f *Fields) Set(name, value string) {
	if f.vals == nil {
		f.vals = map[string]string{}
	}
	if _, seen := f.vals[name]; !seen {
		f.keys = append(f.keys, name)
	}
	f.vals[name] = value
}

// Get is the value of name, "" when it is not set.
func (f Fields) Get(name string) string { return f.vals[name] }

// Has reports whether name is set.
func (f Fields) Has(name string) bool {
	_, ok := f.vals[name]
	return ok
}

// Len is the number of names set.
func (f Fields) Len() int { return len(f.keys) }

// Keys are the names in JavaScript own-key order.
func (f Fields) Keys() []string {
	var indices, names []string
	for _, k := range f.keys {
		if isArrayIndex(k) {
			indices = append(indices, k)
		} else {
			names = append(names, k)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		a, _ := strconv.ParseUint(indices[i], 10, 64)
		b, _ := strconv.ParseUint(indices[j], 10, 64)
		return a < b
	})
	return append(indices, names...)
}

// isArrayIndex is the JavaScript array index test: the canonical decimal form
// of an integer from 0 to 2^32 - 2.
func isArrayIndex(k string) bool {
	n, err := strconv.ParseUint(k, 10, 64)
	return err == nil && n <= 1<<32-2 && strconv.FormatUint(n, 10) == k
}
