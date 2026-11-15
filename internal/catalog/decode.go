package catalog

import (
	"bytes"
	"errors"
	"io"

	yaml "go.yaml.in/yaml/v3"
)

// This is the only file that names the YAML decoder (docs/architecture.md
// section 8). It decodes into generic values, never typed structs: nil, bool,
// int, int64, uint64, float64, string, []any and *Map. Validation runs on
// those values, so `model: 123` gives the oracle's coercion message instead of
// a type error.
//
// Mappings keep their key order (*Map), because the oracle iterates them in
// document order (Object.values on a success entry). Scalars are resolved by
// the decoder itself, except timestamps and binary scalars, which stay strings
// as in the YAML 1.2 core schema the oracle's parser uses. A merge key `<<` is
// an ordinary key, as it is in that schema.

// errMultipleDocuments matches the oracle's parser, which refuses a source
// with more than one document.
var errMultipleDocuments = errors.New("source contains multiple documents")

// decodeYAML parses one YAML document into a generic value. An empty source
// (or one with only comments) is nil, like the oracle's parse of an empty string.
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
