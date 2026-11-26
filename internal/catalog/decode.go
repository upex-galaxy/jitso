package catalog

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	yaml "go.yaml.in/yaml/v3"
)

// This is the only file that names the YAML decoder (docs/architecture.md
// section 8). It decodes into generic values, never typed structs: nil, bool,
// int, int64, uint64, float64, string, []any and *Map. Validation runs on
// those values, so `model: 123` gives the String() coercion message instead of
// a type error.
//
// Mappings keep their key order (*Map), because validation walks them in
// document order. Scalars are resolved by
// the decoder itself, except timestamps and binary scalars, which stay strings
// as in the YAML 1.2 core schema. A merge key `<<` is
// an ordinary key, as it is in that schema.

// errMultipleDocuments is the refusal of a source with more
// than one document.
var errMultipleDocuments = errors.New("source contains multiple documents")

// decodeYAML parses one YAML document into a generic value. An empty source
// (or one with only comments) is nil, like the parse of an empty string.
func decodeYAML(data []byte) (any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errMultipleDocuments
	}
	// A full decode reports what the node walk below does not check, such as a
	// key defined twice in one mapping.
	var check any
	if err := doc.Decode(&check); err != nil {
		return nil, err
	}
	return fromNode(&doc)
}

func fromNode(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return fromNode(n.Content[0])
	case yaml.AliasNode:
		return fromNode(n.Alias)
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := fromNode(c)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.MappingNode:
		m := &Map{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, err := fromNode(n.Content[i])
			if err != nil {
				return nil, err
			}
			val, err := fromNode(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m.set(keyText(key), val)
		}
		return m, nil
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!timestamp", "!!binary":
			return n.Value, nil
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, nil
}

// StatusSpan is the byte range of the text of the top-level `status` scalar of
// a build file: data[start:end] is the value as written, without the quotes of
// a quoted scalar. ok is false when the file has no such scalar, or when its
// text cannot be located with certainty: a value with a tag or an anchor, an
// alias, a block or multi-line scalar, a quoted value that holds an escape. A
// caller that rewrites the value (`build save` turns draft into approved) can
// then change that range and keep every other byte.
//
// The position comes from the YAML node (its line and column), and the text at
// that position is checked against the value the decoder read, so a wrong
// position can never select the wrong bytes.
func StatusSpan(data []byte) (start, end int, ok bool) {
	var doc yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
		return 0, 0, false
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return 0, 0, false
	}
	m := doc.Content[0]
	var val *yaml.Node
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == "status" {
			val = m.Content[i+1]
			break
		}
	}
	if val == nil || val.Kind != yaml.ScalarNode || val.Anchor != "" || val.Style&yaml.TaggedStyle != 0 {
		return 0, 0, false
	}
	at, ok := offsetAt(data, val.Line, val.Column)
	if !ok {
		return 0, 0, false
	}
	text := val.Value
	switch {
	case val.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.FlowStyle) != 0:
		return 0, 0, false
	case val.Style&yaml.DoubleQuotedStyle != 0:
		return quotedSpan(data, at, '"', text)
	case val.Style&yaml.SingleQuotedStyle != 0:
		return quotedSpan(data, at, '\'', text)
	}
	if text == "" || !bytes.HasPrefix(data[at:], []byte(text)) {
		return 0, 0, false
	}
	return at, at + len(text), true
}

// AddStatus is data, a build file with no top-level `status` key, with one
// added and every other byte kept: on a line of its own before the first key,
// with that key's indentation and the file's line break, or right after the
// brace of a flow mapping. ok is false when the file has the key already, or
// the place cannot be found with certainty: the result is decoded again and
// must hold the same mapping plus the status.
func AddStatus(data []byte, status string) ([]byte, bool) {
	old, ok := decodeMap(data)
	if !ok || len(old.keys) == 0 {
		return nil, false
	}
	if _, has := old.Get("status"); has {
		return nil, false
	}
	var doc yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
		return nil, false
	}
	m := doc.Content[0]
	flow := m.Style&yaml.FlowStyle != 0
	node := m
	if !flow {
		node = m.Content[0]
	}
	at, ok := offsetAt(data, node.Line, node.Column)
	if !ok || at >= len(data) {
		return nil, false
	}
	added := "status: " + status
	if flow {
		if data[at] != '{' {
			return nil, false
		}
		at++
		added += ", "
	} else {
		start := lineStart(data, at)
		indent := string(data[start:at])
		if strings.Trim(indent, " ") != "" {
			return nil, false
		}
		at, added = start, indent+added+lineBreak(data)
	}
	out := slices.Concat(data[:at], []byte(added), data[at:])
	got, ok := decodeMap(out)
	if !ok || len(got.keys) != len(old.keys)+1 || got.Value("status") != status {
		return nil, false
	}
	for _, key := range old.keys {
		if !reflect.DeepEqual(old.vals[key], got.vals[key]) {
			return nil, false
		}
	}
	return out, true
}

// decodeMap is decodeYAML of a document that is a mapping.
func decodeMap(data []byte) (*Map, bool) {
	v, err := decodeYAML(data)
	m, ok := v.(*Map)
	return m, err == nil && ok
}

// lineStart is the offset of the first byte of the line that holds data[at],
// after a leading byte order mark.
func lineStart(data []byte, at int) int {
	start := 0
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		start = 3
	}
	for i := start; i < at; {
		if n := breakLen(data[i:]); n > 0 {
			i += n
			start = i
			continue
		}
		i++
	}
	return start
}

// lineBreak is the line break of a file: "\r\n" when its first "\n" follows a
// "\r", else "\n".
func lineBreak(data []byte) string {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// quotedSpan is the span of the text of a quoted scalar that starts at the
// quote at data[at]: the text must be written as is between the quotes.
func quotedSpan(data []byte, at int, quote byte, text string) (int, int, bool) {
	start, end := at+1, at+1+len(text)
	if end >= len(data) || data[at] != quote || data[end] != quote || string(data[start:end]) != text {
		return 0, 0, false
	}
	return start, end, true
}

// offsetAt is the byte offset of the 1-based line and column that the YAML
// parser reports: a column counts characters, and a line break is CR, LF,
// CRLF, NEL, LS or PS. A leading byte order mark is not counted.
func offsetAt(data []byte, line, column int) (int, bool) {
	i := 0
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		i = 3
	}
	for l := 1; l < line; {
		if i >= len(data) {
			return 0, false
		}
		if n := breakLen(data[i:]); n > 0 {
			i += n
			l++
			continue
		}
		_, w := utf8.DecodeRune(data[i:])
		i += w
	}
	for c := 1; c < column; c++ {
		if i >= len(data) || breakLen(data[i:]) > 0 {
			return 0, false
		}
		_, w := utf8.DecodeRune(data[i:])
		i += w
	}
	return i, true
}

// breakLen is the length of the line break at the start of b, 0 when there is
// none.
func breakLen(b []byte) int {
	if bytes.HasPrefix(b, []byte("\r\n")) {
		return 2
	}
	switch r, w := utf8.DecodeRune(b); r {
	case '\r', '\n', '\u0085', '\u2028', '\u2029':
		return w
	}
	return 0
}
