package acpdbg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Direction is the JSONL entry kind.
type Direction string

const (
	DirMeta     Direction = "meta"     // start/close/timeout markers from acpdbg itself
	DirSent     Direction = "sent"     // frames we wrote to the child's stdin
	DirReceived Direction = "received" // frames we read from the child's stdout
	DirStderr   Direction = "stderr"   // lines captured from the child's stderr (opt-in)
)

// Entry is a single line in the JSONL output. Exactly one of Frame / Event /
// Line is set depending on Direction.
type Entry struct {
	TS        string    `json:"ts"`
	Direction Direction `json:"direction"`

	// Meta entries carry an event name + free-form payload.
	Event string         `json:"event,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`

	// Sent/Received entries carry the raw JSON-RPC frame.
	Frame Frame `json:"frame,omitempty"`

	// Stderr entries carry a single captured line.
	Line string `json:"line,omitempty"`
}

// Recorder writes Entries to a JSONL file, thread-safe for concurrent
// stdin/stdout/stderr goroutines.
type Recorder struct {
	path string
	f    *os.File

	mu sync.Mutex
}

// NewRecorder creates a JSONL file at the given path, creating parent
// directories as needed. Overwrites any existing file at the path.
func NewRecorder(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return &Recorder{path: path, f: f}, nil
}

// Path returns the JSONL file path.
func (r *Recorder) Path() string { return r.path }

// Close flushes and closes the underlying file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Meta writes a meta event with an optional payload.
func (r *Recorder) Meta(event string, payload map[string]any) error {
	return r.write(Entry{
		TS:        now(),
		Direction: DirMeta,
		Event:     event,
		Meta:      payload,
	})
}

// Sent writes a frame we sent to the child.
func (r *Recorder) Sent(frame Frame) error {
	return r.write(Entry{
		TS:        now(),
		Direction: DirSent,
		Frame:     frame,
	})
}

// Received writes a frame we received from the child.
func (r *Recorder) Received(frame Frame) error {
	return r.write(Entry{
		TS:        now(),
		Direction: DirReceived,
		Frame:     frame,
	})
}

// Stderr writes a single captured stderr line.
func (r *Recorder) Stderr(line string) error {
	return r.write(Entry{
		TS:        now(),
		Direction: DirStderr,
		Line:      line,
	})
}

func (r *Recorder) write(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	data = append(data, '\n')
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return fmt.Errorf("recorder closed")
	}
	if _, err := r.f.Write(data); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}

func now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}
