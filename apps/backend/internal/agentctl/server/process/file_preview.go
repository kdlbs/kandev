package process

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types"
)

const maxFilePreviewBytes int64 = 64 << 20

var (
	errFilePreviewLimit       = types.ErrFilePreviewLimit
	errFilePreviewChanged     = errors.New("file preview changed")
	errFilePreviewUnavailable = errors.New("file preview unavailable")
)

// filePreview holds a metadata-only selection. Its owner must authorize external
// access before calling CopyTo and close it on cancellation or receipt expiry.
type filePreview struct {
	Path     string
	External bool
	Size     int64
	mu       sync.Mutex
	file     *os.File
	info     os.FileInfo
	selected string
	consumed bool
	check    func() error
}

func prepareFilePreview(root, path string) (*filePreview, error) {
	selected, canonical, external, err := resolveFilePreviewPath(root, path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errFilePreviewUnavailable
	}
	if info.Size() > maxFilePreviewBytes {
		return nil, errFilePreviewLimit
	}
	// Walk canonical parents without following replacement symlinks.
	parent, err := openQuestionImageBase(filepath.Dir(canonical))
	if err != nil {
		return nil, errFilePreviewUnavailable
	}
	defer func() { _ = parent.Close() }()
	file, err := openQuestionImageFile(parent, filepath.Base(canonical))
	if err != nil {
		return nil, errFilePreviewUnavailable
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !samePreviewRevision(info, opened) {
		_ = file.Close()
		return nil, errFilePreviewChanged
	}
	p := &filePreview{Path: canonical, External: external, Size: info.Size(), file: file, info: info, selected: selected}
	if err := p.validate(); err != nil {
		_ = p.Close()
		return nil, err
	}
	return p, nil
}

func resolveFilePreviewPath(root, path string) (selected, canonical string, external bool, err error) {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", false, errFilePreviewUnavailable
		}
		path = filepath.Join(home, path[2:])
	}
	if path == "" || strings.ContainsRune(path, 0) || (!filepath.IsAbs(path) && !filepath.IsLocal(path)) {
		return "", "", false, errFilePreviewUnavailable
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", false, errFilePreviewUnavailable
	}
	selected = path
	if !filepath.IsAbs(selected) {
		selected = filepath.Join(base, selected)
	}
	canonical, err = filepath.EvalSymlinks(selected)
	if err != nil {
		return "", "", false, errFilePreviewUnavailable
	}
	rel, relErr := filepath.Rel(base, canonical)
	external = relErr != nil || pathEscapesRoot(rel)
	return selected, canonical, external, nil
}

func samePreviewRevision(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

func (p *filePreview) validate() error {
	if p.check != nil {
		if err := p.check(); err != nil {
			return err
		}
	}
	canonical, err := filepath.EvalSymlinks(p.selected)
	if err != nil || canonical != p.Path {
		return errFilePreviewChanged
	}
	current, err := os.Stat(canonical)
	if err != nil || !samePreviewRevision(p.info, current) {
		return errFilePreviewChanged
	}
	opened, err := p.file.Stat()
	if err != nil || !samePreviewRevision(p.info, opened) {
		return errFilePreviewChanged
	}
	return nil
}

func (p *filePreview) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	return err
}

func (p *filePreview) CopyTo(ctx context.Context, w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil || p.consumed {
		return errFilePreviewUnavailable
	}
	p.consumed = true
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.validate(); err != nil {
		return err
	}
	reader := &filePreviewReader{ctx: ctx, reader: io.LimitReader(p.file, maxFilePreviewBytes+1)}
	n, err := io.CopyBuffer(w, reader, make([]byte, 32<<10))
	if err != nil {
		return err
	}
	if n > maxFilePreviewBytes {
		return errFilePreviewLimit
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.validate()
}

type filePreviewReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *filePreviewReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}
