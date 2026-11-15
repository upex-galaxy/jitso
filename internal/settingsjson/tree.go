// Package settingsjson edits a Claude Code settings file (.claude/settings.json)
// without disturbing anything the user wrote.
//
// The file is decoded into an order-preserving tree (jsonx.Object for objects,
// []any, json.Number, string, bool and nil), edited there, and encoded again
// with the file's own indent. A file that does not give its own bytes back when
// the unchanged tree is encoded (several keys on a line, an indent that jumps,
// CRLF, a \u escape, a space before a colon) is refused instead of reformatted,
// so an edit never rewrites lines it did not mean to change and `--remove`
// restores the original bytes.
//
// It is pure: bytes in, bytes out. Reading and writing the file is the
// caller's job. Every problem with the file is a *Problem; the caller turns it
// into an exit code and a message that names the file.
package settingsjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// Kind tells what is wrong with a settings file.
type Kind int

// The problem kinds.
const (
	// KindSyntax: the bytes are not JSON. Detail is the decoder's message.
	KindSyntax Kind = iota + 1
	// KindNotObject: the root is not an object.
	KindNotObject
	// KindWrongType: a value jitso walks through has the wrong type. Path is
	// its dot path and Want is "an object" or "an array".
	KindWrongType
	// KindDuplicateKey: an object has the same key twice. Key and Path name it.
	KindDuplicateKey
	// KindLayout: the file does not re-encode to its own bytes.
	KindLayout
)

// Problem is a settings file jitso refuses to edit. Nothing is written when
// one is returned.
type Problem struct {
	Kind   Kind
	Detail string
	Path   string
	Want   string
	Key    string
}

// Error is the message without the file name.
func (p *Problem) Error() string { return p.Message("the settings file") }

// Message is the refusal as one line (the layout one ends with the colon that
// introduces the fragment), for the file shown as file. The text is the
// design's, without the "jitso: " prefix the CLI adds.
func (p *Problem) Message(file string) string {
	switch p.Kind {
	case KindSyntax:
		return fmt.Sprintf("%s is not valid JSON (%s); nothing written", file, p.Detail)
	case KindNotObject:
		return file + " must hold a JSON object; nothing written"
	case KindWrongType:
		return fmt.Sprintf("%q in %s must be %s; nothing written", p.Path, file, p.Want)
	case KindDuplicateKey:
		return fmt.Sprintf("%s has the key %q twice at %q; nothing written", file, p.Key, p.Path)
	case KindLayout:
		return file + " is not laid out the way jitso writes JSON (one value per line, a consistent indent); nothing written. Add these entries by hand, or re-save the file with a 2-space indent and run again:"
	}
	return file + ": unknown problem"
}

// Layout is how a file is laid out: the indent unit of one level ("" for a
// compact file) and whether it ends with a newline.
type Layout struct {
	Indent  string
	Newline bool
}

// DefaultIndent is the indent of a new file.
const DefaultIndent = "  "

// document is a decoded file.
type document struct {
	raw    []byte
	root   jsonx.Object
	layout Layout
}

// parse decodes and checks the shape of doc. Absent files are the caller's
// business (a nil doc never reaches it).
func parse(doc []byte) (*document, *Problem) {
	root, p := decode(doc)
	if p != nil {
		return nil, p
	}
	obj, ok := root.(jsonx.Object)
	if !ok {
		return nil, &Problem{Kind: KindNotObject}
	}
	if p := checkShape(obj); p != nil {
		return nil, p
	}
	return &document{raw: doc, root: obj, layout: detectLayout(doc, obj)}, nil
}

// checkLayout is nil when the unchanged tree encodes to the file's own bytes.
func (d *document) checkLayout() *Problem {
	out, err := d.render(d.root)
	if err != nil || !bytes.Equal(out, d.raw) {
		return &Problem{Kind: KindLayout}
	}
	return nil
}

// render encodes a tree in the document's layout.
func (d *document) render(root jsonx.Object) ([]byte, error) {
	return encode(root, d.layout)
}

// encode writes v with the layout: compact, or one value per line with the
// indent unit, and a trailing newline when the layout has one.
func encode(v any, l Layout) ([]byte, error) {
	var out []byte
	var err error
	if l.Indent == "" {
		out, err = jsonx.Marshal(v)
	} else {
		out, err = jsonx.MarshalIndent(v, l.Indent)
	}
	if err != nil {
		return nil, err
	}
	if l.Newline {
		out = append(out, '\n')
	}
	return out, nil
}

// detectLayout reads the layout from the bytes: the indent unit is the leading
// whitespace of the first indented line, and none means compact (an empty root
// object, which has no line to read it from, gets the default indent).
func detectLayout(doc []byte, root jsonx.Object) Layout {
	l := Layout{Newline: bytes.HasSuffix(doc, []byte("\n"))}
	lines := strings.Split(string(doc), "\n")
	for _, line := range lines[1:] {
		n := 0
		for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
			n++
		}
		if n > 0 {
			l.Indent = line[:n]
			return l
		}
	}
	if len(root) == 0 {
		l.Indent = DefaultIndent
	}
	return l
}

// decode reads doc into the tree. The bytes are validated first, with the
// standard decoder, so a syntax error carries its message and a trailing
// comma, trailing bytes or an empty file never reach the token reader; the
// token pass then only has to find duplicate keys.
func decode(doc []byte) (any, *Problem) {
	var raw json.RawMessage
	if err := json.Unmarshal(doc, &raw); err != nil {
		return nil, &Problem{Kind: KindSyntax, Detail: err.Error()}
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	return readValue(dec, "")
}

func readValue(dec *json.Decoder, path string) (any, *Problem) {
	tok, err := dec.Token()
	if err != nil {
		return nil, &Problem{Kind: KindSyntax, Detail: err.Error()}
	}
	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		return tok, nil // string, json.Number, bool or nil
	}
	if delim == '[' {
		list := []any{}
		for dec.More() {
			v, p := readValue(dec, join(path, strconv.Itoa(len(list))))
			if p != nil {
				return nil, p
			}
			list = append(list, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, &Problem{Kind: KindSyntax, Detail: err.Error()}
		}
		return list, nil
	}
	obj := jsonx.Object{}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, &Problem{Kind: KindSyntax, Detail: err.Error()}
		}
		key, _ := keyTok.(string)
		if seen[key] {
			at := path
			if at == "" {
				at = "."
			}
			return nil, &Problem{Kind: KindDuplicateKey, Key: key, Path: at}
		}
		seen[key] = true
		v, p := readValue(dec, join(path, key))
		if p != nil {
			return nil, p
		}
		obj = append(obj, jsonx.Field{Key: key, Value: v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, &Problem{Kind: KindSyntax, Detail: err.Error()}
	}
	return obj, nil
}

func join(path, part string) string {
	if path == "" {
		return part
	}
	return path + "." + part
}

// checkShape verifies the part of the tree jitso walks: `hooks` is an object,
// every event under it an array, every group an object, and a group's own
// `hooks` an array. Anything else under those paths is the user's and is not
// looked at.
func checkShape(root jsonx.Object) *Problem {
	hv, ok := lookup(root, "hooks")
	if !ok {
		return nil
	}
	hooks, ok := hv.(jsonx.Object)
	if !ok {
		return &Problem{Kind: KindWrongType, Path: "hooks", Want: "an object"}
	}
	for _, ev := range hooks {
		at := "hooks." + ev.Key
		groups, ok := ev.Value.([]any)
		if !ok {
			return &Problem{Kind: KindWrongType, Path: at, Want: "an array"}
		}
		for i, g := range groups {
			gat := join(at, strconv.Itoa(i))
			group, ok := g.(jsonx.Object)
			if !ok {
				return &Problem{Kind: KindWrongType, Path: gat, Want: "an object"}
			}
			if hv, ok := lookup(group, "hooks"); ok {
				if _, ok := hv.([]any); !ok {
					return &Problem{Kind: KindWrongType, Path: gat + ".hooks", Want: "an array"}
				}
			}
		}
	}
	return nil
}

// lookup is the value of key in obj.
func lookup(obj jsonx.Object, key string) (any, bool) {
	for _, f := range obj {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

// with is a copy of obj in which key has the value, in place when the key is
// there and at the end when it is not.
func with(obj jsonx.Object, key string, value any) jsonx.Object {
	out := make(jsonx.Object, 0, len(obj)+1)
	done := false
	for _, f := range obj {
		if f.Key == key {
			out = append(out, jsonx.Field{Key: key, Value: value})
			done = true
			continue
		}
		out = append(out, f)
	}
	if !done {
		out = append(out, jsonx.Field{Key: key, Value: value})
	}
	return out
}

// without is a copy of obj with the key left out.
func without(obj jsonx.Object, key string) jsonx.Object {
	out := make(jsonx.Object, 0, len(obj))
	for _, f := range obj {
		if f.Key != key {
			out = append(out, f)
		}
	}
	return out
}
