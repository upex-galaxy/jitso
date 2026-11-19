package usage

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func msg(id string, in, cw, cr, out int) string {
	return `{"type":"assistant","message":{"id":"` + id + `","usage":{"input_tokens":` + itoa(in) +
		`,"cache_creation_input_tokens":` + itoa(cw) + `,"cache_read_input_tokens":` + itoa(cr) +
		`,"output_tokens":` + itoa(out) + `}}}` + "\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestSum(t *testing.T) {
	long := `{"type":"user","message":{"content":"` + strings.Repeat("x", 200_000) + `"}}` + "\n"
	tests := []struct {
		name string
		in   string
		want Totals
	}{
		{"empty", "", Totals{}},
		{"one message", msg("a", 1, 2, 3, 4), Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Messages: 1}},
		{"last usage of an id wins", msg("a", 1, 2, 3, 1) + msg("a", 1, 2, 3, 9), Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 9, Messages: 1}},
		{"two ids add up", msg("a", 1, 2, 3, 4) + msg("b", 10, 20, 30, 40), Totals{Input: 11, CacheWrite: 22, CacheRead: 33, Output: 44, Messages: 2}},
		{"no trailing newline", strings.TrimSuffix(msg("a", 1, 2, 3, 4), "\n"), Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Messages: 1}},
		{"lines without id or usage add nothing", `{"type":"user","message":{"content":"hi"}}` + "\n" + `{"message":{"usage":{"input_tokens":5}}}` + "\n" + `{"message":{"id":"z"}}` + "\n", Totals{}},
		{"invalid json is skipped", "not json\n" + msg("a", 1, 2, 3, 4) + "{\n", Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Messages: 1}},
		{"blank lines and CRLF", "\n\r\n" + strings.TrimSuffix(msg("a", 1, 2, 3, 4), "\n") + "\r\n", Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Messages: 1}},
		{"a line longer than 64 KiB", long + msg("a", 1, 2, 3, 4), Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Messages: 1}},
		{"missing fields count as zero", `{"message":{"id":"a","usage":{"output_tokens":7}}}` + "\n", Totals{Output: 7, Messages: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(strings.NewReader(tt.in))
			if err != nil || got != tt.want {
				t.Errorf("got %+v err %v, want %+v", got, err, tt.want)
			}
		})
	}
}

type failing struct{}

func (failing) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestSumReadError(t *testing.T) {
	if _, err := Sum(io.MultiReader(strings.NewReader(msg("a", 1, 2, 3, 4)), failing{})); err == nil {
		t.Error("a read error must be returned")
	}
}

func TestAdd(t *testing.T) {
	a := Totals{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Messages: 5, SubagentFiles: 1}
	a.Add(Totals{Input: 10, CacheWrite: 20, CacheRead: 30, Output: 40, Messages: 50, SubagentFiles: 9})
	want := Totals{Input: 11, CacheWrite: 22, CacheRead: 33, Output: 44, Messages: 55, SubagentFiles: 1}
	if a != want {
		t.Errorf("got %+v, want %+v", a, want)
	}
}
