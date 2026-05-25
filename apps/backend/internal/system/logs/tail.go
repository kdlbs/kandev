package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/common/logger/buffer"
)

// tailChunkSize is the read window stepped backward from the end of file.
const tailChunkSize = 4 * 1024

// Tail returns the last n lines of the most recent log activity in order
// (oldest first, newest last). When a log file is configured and non-empty
// it is read backward in fixed-size chunks so memory usage stays bounded.
// When no file is available (stdout-only logger, file missing, or file
// empty) Tail falls back to the in-memory ring buffer so the UI never shows
// "no logs" for a healthy running process.
func (s *Service) Tail(n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	lines, err := s.tailFromFile(n)
	if err != nil {
		return nil, err
	}
	if len(lines) > 0 {
		return lines, nil
	}
	return s.tailFromBuffer(n), nil
}

// tailFromFile reads the active log file from disk and returns its tail.
// Returns an empty slice (no error) when there is no file to read.
func (s *Service) tailFromFile(n int) ([]string, error) {
	if s.logDir == "" || s.currentName == "" {
		return []string{}, nil
	}
	path := filepath.Join(s.logDir, s.currentName)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	if size == 0 {
		return []string{}, nil
	}
	return tailReader(f, size, n)
}

// tailFromBuffer returns up to n formatted lines from the shared in-memory
// ring buffer. Each entry becomes a single text line in zap's "console"
// format: "<timestamp> <LEVEL> <caller> <message> [field=value ...]".
func (s *Service) tailFromBuffer(n int) []string {
	if s.memBuffer == nil {
		return []string{}
	}
	entries := s.memBuffer.Snapshot()
	if len(entries) == 0 {
		return []string{}
	}
	start := 0
	if len(entries) > n {
		start = len(entries) - n
	}
	out := make([]string, 0, len(entries)-start)
	for _, e := range entries[start:] {
		out = append(out, formatBufferEntry(e))
	}
	return out
}

// formatBufferEntry renders one ring-buffer entry as a single human-readable
// line. Fields are appended as "key=value" pairs, JSON-encoded for non-string
// values so the line stays single-row in the viewer.
func formatBufferEntry(e buffer.Entry) string {
	var b strings.Builder
	b.WriteString(e.Timestamp.Format("2006-01-02T15:04:05.000Z"))
	b.WriteString(" ")
	b.WriteString(strings.ToUpper(e.Level))
	if e.Caller != "" {
		b.WriteString(" ")
		b.WriteString(e.Caller)
	}
	b.WriteString(" ")
	b.WriteString(e.Message)
	if len(e.Fields) > 0 {
		keys := make([]string, 0, len(e.Fields))
		for k := range e.Fields {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, " %s=%s", k, formatFieldValue(e.Fields[k]))
		}
	}
	if e.Stack != "" {
		b.WriteString(" stack=...")
	}
	return b.String()
}

func formatFieldValue(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	default:
		if data, err := json.Marshal(typed); err == nil {
			return string(data)
		}
		return fmt.Sprintf("%v", typed)
	}
}

// tailReader reads f backward in tailChunkSize windows until n line breaks
// are accumulated or the start of file is reached.
func tailReader(f io.ReaderAt, size int64, n int) ([]string, error) {
	var (
		buf    []byte
		offset = size
	)
	for offset > 0 {
		readSize := int64(tailChunkSize)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		chunk := make([]byte, readSize)
		if _, err := f.ReadAt(chunk, offset); err != nil && err != io.EOF {
			return nil, err
		}
		buf = append(chunk, buf...)
		if countNewlines(buf) > n {
			break
		}
	}
	return lastNLines(buf, n), nil
}

// countNewlines returns the number of '\n' bytes in b.
func countNewlines(b []byte) int {
	count := 0
	for _, c := range b {
		if c == '\n' {
			count++
		}
	}
	return count
}

// lastNLines splits b on '\n' and returns at most the last n lines, preserving
// order. A file that does not end with '\n' still yields its final partial
// line as the last entry.
func lastNLines(b []byte, n int) []string {
	if len(b) == 0 {
		return []string{}
	}
	// Strip a single trailing newline so we don't emit a phantom empty line.
	trimmed := b
	if trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	// Walk from the end collecting up to n newline-delimited lines.
	out := make([]string, 0, n)
	end := len(trimmed)
	for i := len(trimmed) - 1; i >= 0 && len(out) < n; i-- {
		if trimmed[i] == '\n' {
			out = append(out, string(trimmed[i+1:end]))
			end = i
		}
	}
	if len(out) < n && end > 0 {
		out = append(out, string(trimmed[:end]))
	}
	// out is in reverse order; flip in place.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
