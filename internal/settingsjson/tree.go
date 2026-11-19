// Package settingsjson edits a Claude Code settings file (.claude/settings.json)
// without disturbing anything the user wrote.
//
// The file is decoded into an order-preserving tree (jsonx.Object for objects,
// []any, json.Number, string, bool and nil) that also records the byte span of
// every root member. An edit changes only the root member `hooks`: its new
// value is encoded at depth 1 with the file's own indent and spliced over the
// old member (or inserted after the last root member), so every other byte of
// the file stays as written. An old `hooks` member that does not give its own
// bytes back when encoded (a group on one line, an inline array, a \u escape),
// and a file with CRLF line ends, are refused instead of reformatted, so an
// edit never rewrites lines it did not mean to change and `--remove` restores
// the original bytes.
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
	// KindLayout: the part of the file an edit rewrites does not re-encode to
	// its own bytes.
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

// document is a decoded file. members holds the span of each field of root,
// in the same order.
type document struct {
	raw     []byte
	root    jsonx.Object
	layout  Layout
	members []span
}

// span is the bytes of one root member: from the opening quote of its key to
// the end of its value.
type span struct{ start, end int }

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
	members, err := rootMembers(doc)
	if err != nil {
		return nil, &Problem{Kind: KindSyntax, Detail: err.Error()}
	}
	return &document{raw: doc, root: obj, layout: detectLayout(doc, obj), members: members}, nil
}

// rootMembers is the span of every member of the root object of a valid
// document, in file order.
func rootMembers(doc []byte) ([]span, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var out []span
	for dec.More() {
		start := int(dec.InputOffset())
		for start < len(doc) && strings.IndexByte(" \t\r\n,", doc[start]) >= 0 {
			start++
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, span{start: start, end: int(dec.InputOffset())})
	}
	return out, nil
}

// setHooks is the file with the root member `hooks` set to hooks, or taken
// out when hooks is empty; nil means nothing is left and the file goes. Only
// that member is rewritten, and only when its old bytes are what jitso would
// write for its old value; an absent member is inserted after the last root
// member. An empty root has no member to splice after, so it is rendered
// whole, as before.
func (d *document) setHooks(hooks jsonx.Object) ([]byte, *Problem) {
	if d.raw == nil || len(d.root) == 0 {
		if d.raw != nil {
			if p := d.checkLayout(); p != nil {
				return nil, p
			}
		}
		out, err := d.render(with(d.root, "hooks", hooks))
		if err != nil {
			return nil, &Problem{Kind: KindLayout}
		}
		return out, nil
	}
	if bytes.IndexByte(d.raw, '\r') >= 0 {
		return nil, &Problem{Kind: KindLayout}
	}
	if d.layout.Indent == "" && bytes.IndexByte(bytes.TrimSuffix(d.raw, []byte("\n")), '\n') >= 0 {
		return nil, &Problem{Kind: KindLayout}
	}
	at := -1
	for i, f := range d.root {
		if f.Key == "hooks" {
			at = i
		}
	}
	if at < 0 {
		text, err := d.member("hooks", hooks)
		if err != nil {
			return nil, &Problem{Kind: KindLayout}
		}
		sep := ",\n" + d.layout.Indent
		if d.layout.Indent == "" {
			sep = ","
		}
		end := d.members[len(d.members)-1].end
		return splice(d.raw, end, end, append([]byte(sep), text...)), nil
	}
	m := d.members[at]
	old, err := d.member("hooks", d.root[at].Value)
	if err != nil || !bytes.Equal(old, d.raw[m.start:m.end]) {
		return nil, &Problem{Kind: KindLayout}
	}
	if len(hooks) == 0 {
		switch {
		case len(d.root) == 1:
			return nil, nil
		case at > 0:
			return splice(d.raw, d.members[at-1].end, m.end, nil), nil
		default:
			return splice(d.raw, m.start, d.members[at+1].start, nil), nil
		}
	}
	text, err := d.member("hooks", hooks)
	if err != nil {
		return nil, &Problem{Kind: KindLayout}
	}
	return splice(d.raw, m.start, m.end, text), nil
}

// member is a root member as jitso writes it in the document's layout: the
// key, the colon and the value encoded at depth 1.
func (d *document) member(key string, value any) ([]byte, error) {
	k, err := jsonx.Marshal(key)
	if err != nil {
		return nil, err
	}
	if d.layout.Indent == "" {
		v, err := jsonx.Marshal(value)
		if err != nil {
			return nil, err
		}
		return append(append(k, ':'), v...), nil
	}
	v, err := jsonx.MarshalIndent(value, d.layout.Indent)
	if err != nil {
		return nil, err
	}
	v = bytes.ReplaceAll(v, []byte("\n"), []byte("\n"+d.layout.Indent))
	return append(append(k, ':', ' '), v...), nil
}

// splice is doc with the bytes from start to end replaced by text.
func splice(doc []byte, start, end int, text []byte) []byte {
	out := make([]byte, 0, len(doc)-(end-start)+len(text))
	out = append(out, doc[:start]...)
	out = append(out, text...)
	return append(out, doc[end:]...)
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
