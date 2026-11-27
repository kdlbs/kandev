package codexdbg

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/codexappserver"
)

const (
	defaultMaxCaptureBytes = 100 * 1024 * 1024
	reservedCaptureBytes   = 512
	maxCaptureLineBytes    = 21 * 1024 * 1024
	captureEventClose      = "close"
)

var ErrCaptureLimit = errors.New("capture size limit reached")

// CaptureEntry is one validated record from a debugger JSONL capture.
type CaptureEntry struct {
	Sequence   uint64                        `json:"sequence"`
	Timestamp  string                        `json:"timestamp"`
	Kind       string                        `json:"kind"`
	Direction  codexappserver.FrameDirection `json:"direction,omitempty"`
	Event      string                        `json:"event,omitempty"`
	RequestID  json.RawMessage               `json:"request_id,omitempty"`
	Method     string                        `json:"method,omitempty"`
	ResponseTo string                        `json:"response_to,omitempty"`
	Frame      json.RawMessage               `json:"frame,omitempty"`
	Line       string                        `json:"line,omitempty"`
	Meta       map[string]any                `json:"meta,omitempty"`
}

// Recorder writes ordered protocol frames to an owner-only JSONL file.
type Recorder struct {
	mu                 sync.Mutex
	file               *os.File
	path               string
	sequence           uint64
	bytesWritten       int64
	maxBytes           int64
	truncated          bool
	closed             bool
	closeOnce          sync.Once
	closeErr           error
	pendingNames       map[string]string
	pendingServerNames map[string]string
}

// NewRecorder creates a new capture file without following or overwriting an
// existing path. New directories and the capture file are private to the user.
func NewRecorder(path, executableVersion string) (*Recorder, error) {
	return NewRecorderWithLimit(path, executableVersion, defaultMaxCaptureBytes)
}

// NewRecorderWithLimit creates a private capture with an enforced total byte limit.
func NewRecorderWithLimit(path, executableVersion string, maxBytes int64) (*Recorder, error) {
	if path == "" {
		return nil, errors.New("capture path is required")
	}
	if maxBytes < reservedCaptureBytes*2 {
		return nil, fmt.Errorf("capture size limit must be at least %d bytes", reservedCaptureBytes*2)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create capture directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create capture file: %w", err)
	}
	recorder := &Recorder{
		file:               file,
		path:               path,
		maxBytes:           maxBytes,
		pendingNames:       make(map[string]string),
		pendingServerNames: make(map[string]string),
	}
	if err := recorder.writeLocked(CaptureEntry{
		Kind:  "meta",
		Event: "start",
		Meta: map[string]any{
			"executable_version": executableVersion,
			"schema_version":     codexappserver.SupportedCodexVersion,
			"schema_sha256":      codexappserver.SchemaSHA256V0154,
		},
	}); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return recorder, nil
}

// Path returns the file receiving capture entries.
func (r *Recorder) Path() string { return r.path }

// RecordFrame records one complete raw JSON-RPC frame and correlates response
// frames with the method that opened the request.
func (r *Recorder) RecordFrame(direction codexappserver.FrameDirection, frame json.RawMessage) error {
	if !json.Valid(frame) {
		return errors.New("cannot record invalid JSON-RPC frame")
	}
	var envelope struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return fmt.Errorf("decode capture frame: %w", err)
	}
	entry := CaptureEntry{
		Kind:      "frame",
		Direction: direction,
		Method:    envelope.Method,
		RequestID: append(json.RawMessage(nil), envelope.ID...),
		Frame:     append(json.RawMessage(nil), frame...),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("capture is closed")
	}
	key := captureIDKey(envelope.ID)
	if key != "" {
		switch {
		case direction == codexappserver.FrameSent && envelope.Method != "":
			r.pendingNames[key] = envelope.Method
		case direction == codexappserver.FrameReceived && envelope.Method != "":
			r.pendingServerNames[key] = envelope.Method
		case direction == codexappserver.FrameReceived:
			entry.ResponseTo = r.pendingNames[key]
			delete(r.pendingNames, key)
		case direction == codexappserver.FrameSent:
			entry.ResponseTo = r.pendingServerNames[key]
			delete(r.pendingServerNames, key)
		}
	}
	return r.writeLocked(entry)
}

// RecordStderr stores one child stderr line. Call it only when the developer
// explicitly requested stderr capture.
func (r *Recorder) RecordStderr(line string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("capture is closed")
	}
	return r.writeLocked(CaptureEntry{Kind: "stderr", Line: line})
}

// RecordMeta adds a local diagnostic marker that contains no raw protocol
// payload.
func (r *Recorder) RecordMeta(event string, fields map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("capture is closed")
	}
	return r.writeLocked(CaptureEntry{Kind: "meta", Event: event, Meta: fields})
}

// Close writes the operation's close reason and flushes the capture. Repeated
// calls return the same result.
func (r *Recorder) Close(reason string) error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.closed {
			r.closeErr = r.writeLocked(CaptureEntry{Kind: "meta", Event: captureEventClose, Meta: map[string]any{"reason": reason}})
			r.closed = true
		}
		if r.file != nil {
			r.closeErr = errors.Join(r.closeErr, r.file.Sync(), r.file.Close())
			r.file = nil
		}
	})
	return r.closeErr
}

func (r *Recorder) writeLocked(entry CaptureEntry) error {
	if r.file == nil {
		return errors.New("capture file is closed")
	}
	r.sequence++
	entry.Sequence = r.sequence
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(entry)
	if err != nil {
		r.sequence--
		return fmt.Errorf("encode capture entry: %w", err)
	}
	encoded = append(encoded, '\n')
	if r.maxBytes > 0 && r.bytesWritten+int64(len(encoded))+reservedCaptureBytes > r.maxBytes && entry.Event != "capture_truncated" && entry.Event != captureEventClose {
		r.sequence--
		if !r.truncated {
			r.truncated = true
			marker := CaptureEntry{Kind: "meta", Event: "capture_truncated", Meta: map[string]any{"limit_bytes": r.maxBytes}}
			if err := r.writeLocked(marker); err != nil {
				return errors.Join(ErrCaptureLimit, err)
			}
		}
		return ErrCaptureLimit
	}
	if _, err := r.file.Write(encoded); err != nil {
		r.sequence--
		return fmt.Errorf("write capture entry: %w", err)
	}
	r.bytesWritten += int64(len(encoded))
	return nil
}

func captureIDKey(id json.RawMessage) string {
	if len(id) == 0 || bytes.Equal(id, []byte("null")) {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return "s:" + typed
	case json.Number:
		return "n:" + typed.String()
	default:
		return ""
	}
}

// ReadCapture reads a bounded JSONL capture and rejects malformed entries.
func ReadCapture(path string) ([]CaptureEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open capture: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxCaptureLineBytes)
	entries := make([]CaptureEntry, 0, 64)
	for scanner.Scan() {
		var entry CaptureEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode capture entry %d: %w", len(entries)+1, err)
		}
		if entry.Sequence != uint64(len(entries)+1) || entry.Timestamp == "" || entry.Kind == "" {
			return nil, fmt.Errorf("capture entry %d has invalid identity metadata", len(entries)+1)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read capture: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("capture is empty")
	}
	return entries, nil
}
