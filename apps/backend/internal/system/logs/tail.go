package logs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// tailChunkSize is the read window stepped backward from the end of file.
const tailChunkSize = 4 * 1024

// Tail returns the last n lines of the current log file in order (oldest of
// the tail first, newest last). When the log directory or current filename is
// not configured, when n <= 0, or when the file does not exist, Tail returns
// an empty slice and no error. The file is read backward in fixed-size chunks
// so memory usage stays bounded for arbitrarily large logs.
func (s *Service) Tail(n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
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
