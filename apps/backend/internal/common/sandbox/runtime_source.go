package sandbox

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// RuntimeSource selects bounded files below one attempt's private state root.
type RuntimeSource struct {
	Key       string
	Kind      string
	Directory string
	Suffix    string
	MaxDepth  uint32
	MaxFiles  uint32
	MaxBytes  uint32
}

type RuntimeRecord struct {
	Name    string
	Content []byte
}

func (s RuntimeSource) Validate() error {
	if s.Key == "" || len(s.Key) > 128 || strings.ContainsFunc(s.Key, unicode.IsControl) || (s.Kind != "identity" && s.Kind != "audit" && s.Kind != "status") {
		return ErrInvalidPolicy
	}
	if !validSourceLocation(s) || !validSourceLimits(s) {
		return ErrInvalidPolicy
	}
	return nil
}

func ReadRuntimeSource(ctx context.Context, stateDir string, source RuntimeSource) ([]RuntimeRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validRuntimeDirectory(stateDir) || source.Validate() != nil {
		return nil, ErrInvalidPolicy
	}
	directory := filepath.Join(stateDir, filepath.FromSlash(source.Directory))
	canonical, err := filepath.EvalSymlinks(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || canonical != directory {
		return nil, ErrInvalidPolicy
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	defer func() { _ = root.Close() }()
	reader := runtimeSourceReader{root: root, source: source}
	if err = reader.walk(ctx, source.Directory, 1); err != nil {
		return nil, err
	}
	return reader.records, nil
}

type runtimeSourceReader struct {
	root    *os.Root
	source  RuntimeSource
	records []RuntimeRecord
	entries int
	bytes   int
}

func (r *runtimeSourceReader) walk(ctx context.Context, directory string, depth uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := r.root.Open(filepath.FromSlash(directory))
	if err != nil {
		return ErrInvalidPolicy
	}
	entries, err := dir.ReadDir(1025)
	_ = dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return ErrInvalidPolicy
	}
	r.entries += len(entries)
	if r.entries > 1024 {
		return ErrInvalidPolicy
	}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrInvalidPolicy
		}
		name := path.Join(directory, entry.Name())
		if err = r.readEntry(ctx, name, entry, depth); err != nil {
			return err
		}
	}
	return nil
}

func (r *runtimeSourceReader) readEntry(ctx context.Context, name string, entry fs.DirEntry, depth uint32) error {
	if entry.IsDir() {
		if depth < r.source.MaxDepth {
			return r.walk(ctx, name, depth+1)
		}
		return nil
	}
	if !strings.HasSuffix(entry.Name(), r.source.Suffix) {
		return nil
	}
	if len(r.records) >= int(r.source.MaxFiles) {
		return ErrInvalidPolicy
	}
	content, err := r.read(name)
	if err != nil {
		return err
	}
	r.bytes += len(content)
	if r.bytes > 1<<20 {
		return ErrInvalidPolicy
	}
	r.records = append(r.records, RuntimeRecord{Name: name, Content: content})
	return nil
}

func (r *runtimeSourceReader) read(name string) ([]byte, error) {
	info, err := r.root.Lstat(filepath.FromSlash(name))
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(r.source.MaxBytes) {
		return nil, ErrInvalidPolicy
	}
	file, err := r.root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, ErrInvalidPolicy
	}
	content, err := io.ReadAll(io.LimitReader(file, int64(r.source.MaxBytes)+1))
	if err != nil || len(content) > int(r.source.MaxBytes) {
		return nil, ErrInvalidPolicy
	}
	return content, nil
}

func validSourceLocation(source RuntimeSource) bool {
	directory := source.Directory
	if directory == "" || directory == "." || directory == ".." || len(directory) > 255 || path.Clean(directory) != directory || path.IsAbs(directory) || strings.HasPrefix(directory, "../") || strings.ContainsAny(directory, "\\:") || strings.ContainsFunc(directory, unicode.IsControl) {
		return false
	}
	return source.Suffix != "" && len(source.Suffix) <= 64 && !strings.ContainsAny(source.Suffix, "/\\:") && !strings.ContainsFunc(source.Suffix, unicode.IsControl)
}

func validSourceLimits(source RuntimeSource) bool {
	return source.MaxDepth > 0 && source.MaxDepth <= 4 && source.MaxFiles > 0 && source.MaxFiles <= 256 && source.MaxBytes > 0 && source.MaxBytes <= 1<<20
}
