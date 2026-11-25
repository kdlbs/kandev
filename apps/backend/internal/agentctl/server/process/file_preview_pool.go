package process

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

type filePreviewEntry struct {
	file     *filePreview
	expires  time.Time
	timer    *time.Timer
	consumed bool
	cancel   context.CancelFunc
}

type filePreviewPool struct {
	mu        sync.Mutex
	entries   map[string]*filePreviewEntry
	closed    bool
	preparing int
}

func newFilePreviewPool() *filePreviewPool {
	return &filePreviewPool{entries: make(map[string]*filePreviewEntry)}
}

func (p *filePreviewPool) add(file *filePreview) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(p.entries) >= 4 {
		return "", errFilePreviewUnavailable
	}
	token := uuid.NewString()
	entry := &filePreviewEntry{file: file, expires: time.Now().Add(5 * time.Minute)}
	p.entries[token] = entry
	entry.timer = time.AfterFunc(5*time.Minute, func() { p.remove(token) })
	return token, nil
}

func (p *filePreviewPool) remove(token string) {
	p.mu.Lock()
	entry := p.entries[token]
	delete(p.entries, token)
	if entry != nil {
		entry.timer.Stop()
		if entry.cancel != nil {
			entry.cancel()
		}
	}
	p.mu.Unlock()
	if entry != nil {
		_ = entry.file.Close()
	}
}

func (p *filePreviewPool) copyTo(ctx context.Context, token string, w io.Writer) error {
	p.mu.Lock()
	entry := p.entries[token]
	if p.closed || entry == nil || entry.consumed || !time.Now().Before(entry.expires) {
		p.mu.Unlock()
		return errFilePreviewUnavailable
	}
	entry.consumed = true
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	entry.cancel = cancel
	p.mu.Unlock()
	defer p.remove(token)
	return entry.file.CopyTo(readCtx, w)
}

func (p *filePreviewPool) close() {
	p.mu.Lock()
	p.closed = true
	tokens := make([]string, 0, len(p.entries))
	for token := range p.entries {
		tokens = append(tokens, token)
	}
	p.mu.Unlock()
	for _, token := range tokens {
		p.remove(token)
	}
}

func (p *filePreviewPool) reserve() (func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(p.entries)+p.preparing >= 4 {
		return nil, errFilePreviewUnavailable
	}
	p.preparing++
	return func() { p.mu.Lock(); p.preparing--; p.mu.Unlock() }, nil
}
