package instructions

import "bytes"

// unitOf is the line break the file uses: "\r\n" when its first "\n" follows a
// "\r", else "\n". A file with no "\n" that ends in a lone "\r" counts as CRLF,
// because the "\n" jitso adds after it makes that "\r" part of a CRLF.
func unitOf(data []byte) string {
	i := bytes.IndexByte(data, '\n')
	switch {
	case i > 0 && data[i-1] == '\r':
		return "\r\n"
	case i < 0 && len(data) > 0 && data[len(data)-1] == '\r':
		return "\r\n"
	}
	return "\n"
}

// Insert adds the block at the end of a file that has none. old is the
// content now: nil is a file that does not exist, an empty slice an empty
// file. The line break unit is the file's own (CRLF or LF).
//
//	absent                   block + break                              added=new
//	empty                    block + break                              added=0
//	ends with a break        old + break + block + break                added=1
//	no break at the end      old + break + break + block + break        added=2
func Insert(old []byte) []byte {
	unit := unitOf(old)
	var added, lead string
	switch {
	case old == nil:
		added = AddedNew
	case len(old) == 0:
		added = Added0
	case old[len(old)-1] == '\n':
		added, lead = Added1, unit
	default:
		added, lead = Added2, unit+unit
	}
	out := make([]byte, 0, len(old)+len(lead)+512)
	out = append(out, old...)
	out = append(out, lead...)
	out = append(out, Render(added, unit)...)
	out = append(out, unit...)
	return out
}

// Replace rewrites the lines from BEGIN to END of b with the current block,
// keeping the block's `added` value, the file's line break unit and every
// byte outside those lines. It returns data itself (the same slice) when the
// block is already what would be written.
func Replace(data []byte, b *Block) []byte {
	added := b.Added
	if added == "" {
		added = Added0
	}
	want := Render(added, unitOf(data))
	if string(data[b.Start:b.End]) == want {
		return data
	}
	out := make([]byte, 0, len(data)+len(want))
	out = append(out, data[:b.Start]...)
	out = append(out, want...)
	out = append(out, data[b.End:]...)
	return out
}

// Ensure is what `setup instructions` does to a file: insert the block when
// the file has none, replace it in place when it differs, nothing when it is
// current (the result is then data itself). A file that has no block and ends
// inside a fenced code block is refused, because the block would be quoted
// and the next run would add another.
func Ensure(data []byte) ([]byte, error) {
	s := scan(data)
	b, err := findIn(s)
	if err != nil {
		return nil, err
	}
	if b != nil {
		return Replace(data, b), nil
	}
	if s.openFence > 0 {
		return nil, &Problem{Kind: OpenFence, Line: s.openFence}
	}
	return Insert(data), nil
}

// Removal is the result of Remove.
type Removal struct {
	// Found is whether the file had a block.
	Found bool
	// Out is the file without the block (data itself when nothing was found).
	Out []byte
	// Delete is true when jitso created the file (added=new) and nothing else
	// is left in it: the file is to be deleted, not rewritten.
	Delete bool
}

// Remove takes the block out so that the file gets back the bytes it had
// before Insert: it deletes from the start of the BEGIN line through the END
// line and its break, then up to `added` line breaks immediately before (only
// whole breaks of the file's unit, stopping at the first other byte). A block
// whose `added` is 0 or new takes only its own lines with it (a block moved by
// hand, or one at the start of a file jitso created).
func Remove(data []byte) (Removal, error) {
	b, err := Find(data)
	if err != nil {
		return Removal{}, err
	}
	if b == nil {
		return Removal{Out: data}, nil
	}
	unit := []byte(unitOf(data))
	start := b.Start
	n := 0
	switch b.Added {
	case Added1:
		n = 1
	case Added2:
		n = 2
	}
	for ; n > 0 && bytes.HasSuffix(data[:start], unit); n-- {
		start -= len(unit)
	}
	out := make([]byte, 0, len(data)-(b.After-start))
	out = append(out, data[:start]...)
	out = append(out, data[b.After:]...)
	return Removal{Found: true, Out: out, Delete: b.Added == AddedNew && len(out) == 0}, nil
}
