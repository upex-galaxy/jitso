package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// File names inside the .jitso/ folder.
const (
	FileName = "launches.jsonl"
	LockName = "launches.lock"
)

// Default lock timings (the oracle's).
const (
	DefaultLockTimeout = 5 * time.Second
	DefaultStaleAge    = 10 * time.Second
)

// Store is the registry seam: append one event, read every valid event.
type Store interface {
	Append(Record) error
	Events() ([]Event, error)
}

// FileStore is the registry in a .jitso/ folder.
type FileStore struct {
	// Dir is the .jitso/ folder of the primary checkout.
	Dir string
	// LockTimeout is how long Append waits for a busy lock.
	LockTimeout time.Duration
	// StaleAge is the age after which a lock is a dead writer's and is taken over.
	StaleAge time.Duration
	// Now and Sleep are the clock; tests pass a fake one.
	Now   func() time.Time
	Sleep func(time.Duration)
}

// NewFileStore is a FileStore on dir with the default timings and the real clock.
func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir, LockTimeout: DefaultLockTimeout, StaleAge: DefaultStaleAge, Now: time.Now, Sleep: time.Sleep}
}

// Path is the registry file.
func (s *FileStore) Path() string { return filepath.Join(s.Dir, FileName) }

// Encode is the line Append writes for r, validated with the reader's
// validator (so a writer can never write a line readers skip), without the
// trailing newline.
func Encode(r Record) ([]byte, error) {
	r.stamp()
	line, err := jsonx.Marshal(r)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(line, &decoded); err != nil {
		return nil, err
	}
	if problems := Validate(decoded); len(problems) > 0 {
		return nil, fmt.Errorf("invalid %s event: %s", r.kind(), strings.Join(problems, "; "))
	}
	return line, nil
}

// Append validates r and writes it as one line under the lock. It fails on an
// invalid event or a lock that stays busy.
func (s *FileStore) Append(r Record) error {
	line, err := Encode(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	lock := filepath.Join(s.Dir, LockName)
	if err := s.acquire(lock); err != nil {
		return err
	}
	defer os.RemoveAll(lock)
	f, err := os.OpenFile(s.Path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

func (s *FileStore) acquire(lock string) error {
	now, sleep := s.Now, s.Sleep
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	deadline := now().Add(s.LockTimeout)
	for attempt := 0; ; attempt++ {
		err := os.Mkdir(lock, 0o755)
		if err == nil {
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		st, err := os.Stat(lock)
		if err != nil {
			continue // the holder released it between our mkdir and our stat
		}
		if now().Sub(st.ModTime()) > s.StaleAge {
			os.RemoveAll(lock)
			continue
		}
		if !now().Before(deadline) {
			return fmt.Errorf("the registry lock %s stayed busy for %d ms", lock, s.LockTimeout.Milliseconds())
		}
		sleep(time.Duration(min(5+attempt*2, 25)) * time.Millisecond)
	}
}

// Events returns every valid event in file order. A missing file is no event;
// an empty line, a line that is not JSON or an invalid event is skipped.
func (s *FileStore) Events() ([]Event, error) {
	data, err := os.ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}

// CountV1 is the number of lines of registry text that are one JSON object with
// `v` equal to 1: the lines the TypeScript era wrote, which Parse skips. Doctor
// reports them apart from the other invalid lines.
func CountV1(data []byte) int {
	n := 0
	for _, line := range bytes.Split(data, []byte("\n")) {
		var event map[string]any
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if v, ok := event["v"].(float64); ok && v == 1 {
			n++
		}
	}
	return n
}

// Parse reads registry text: the valid events of its lines, in order.
func Parse(data []byte) []Event {
	var events []Event
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(line, &v); err != nil {
			continue
		}
		if len(Validate(v)) > 0 {
			continue
		}
		events = append(events, Event(v.(map[string]any)))
	}
	return events
}
