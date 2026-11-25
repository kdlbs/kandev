package plugins

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

type filePreviewSource struct {
	Read  func(context.Context, io.Writer) error
	Close func()
}

type filePreviewGrant struct {
	scope    filePreviewScope
	revision uint64
	ask      bool
	source   *filePreviewSource
	timer    *time.Timer
	expires  time.Time
	consumed bool
	cancel   context.CancelFunc
}

type filePreviewGrants struct {
	mu          sync.Mutex
	entries     map[string]*filePreviewGrant
	closed      bool
	active      map[string]int
	activeTotal int
	preparing   map[string]int
}

func newFilePreviewGrants() *filePreviewGrants {
	return &filePreviewGrants{entries: make(map[string]*filePreviewGrant), active: make(map[string]int), preparing: make(map[string]int)}
}

func (g *filePreviewGrants) add(scope filePreviewScope, revision uint64, ask bool, source *filePreviewSource) (string, error) {
	if _, err := scope.key(); err != nil {
		return "", err
	}
	if source == nil || source.Read == nil || source.Close == nil {
		return "", errFilePreviewPolicy
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for _, entry := range g.entries {
		if entry.scope.UserID == scope.UserID {
			count++
		}
	}
	if g.closed || count >= 4 || len(g.entries) >= 64 {
		return "", errFilePreviewPolicy
	}
	token := uuid.NewString()
	entry := &filePreviewGrant{scope: scope, revision: revision, ask: ask, source: source, expires: time.Now().Add(5 * time.Minute)}
	g.entries[token] = entry
	entry.timer = time.AfterFunc(5*time.Minute, func() { g.remove(token) })
	return token, nil
}

func (g *filePreviewGrants) read(ctx context.Context, token string, scope filePreviewScope, revision uint64, approved bool, w io.Writer) error {
	g.mu.Lock()
	entry := g.entries[token]
	if g.closed || entry == nil || entry.consumed || entry.scope != scope || entry.revision != revision || !time.Now().Before(entry.expires) || (entry.ask && !approved) {
		g.mu.Unlock()
		return errFilePreviewPolicy
	}
	if g.active[scope.UserID] >= 1 || g.activeTotal >= 4 {
		g.mu.Unlock()
		return errFilePreviewPolicy
	}
	g.active[scope.UserID]++
	g.activeTotal++
	entry.consumed = true
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	entry.cancel = cancel
	g.mu.Unlock()
	defer func() {
		g.remove(token)
		g.mu.Lock()
		g.active[scope.UserID]--
		if g.active[scope.UserID] == 0 {
			delete(g.active, scope.UserID)
		}
		g.activeTotal--
		g.mu.Unlock()
	}()
	return entry.source.Read(readCtx, w)
}

func (g *filePreviewGrants) remove(token string) {
	g.mu.Lock()
	entry := g.entries[token]
	delete(g.entries, token)
	if entry != nil {
		entry.timer.Stop()
		if entry.cancel != nil {
			entry.cancel()
		}
	}
	g.mu.Unlock()
	if entry != nil {
		entry.source.Close()
	}
}

func (g *filePreviewGrants) close() {
	g.mu.Lock()
	g.closed = true
	tokens := make([]string, 0, len(g.entries))
	for token := range g.entries {
		tokens = append(tokens, token)
	}
	g.mu.Unlock()
	for _, token := range tokens {
		g.remove(token)
	}
}

func (g *filePreviewGrants) cancelOwned(token string, scope filePreviewScope) {
	g.mu.Lock()
	entry := g.entries[token]
	owned := entry != nil && entry.scope == scope
	g.mu.Unlock()
	if owned {
		g.remove(token)
	}
}

func (g *filePreviewGrants) revokeScope(scope filePreviewScope) {
	g.mu.Lock()
	var tokens []string
	for token, entry := range g.entries {
		if entry.scope.UserID == scope.UserID && entry.scope.WorkspaceID == scope.WorkspaceID && entry.scope.InstallationID == scope.InstallationID {
			tokens = append(tokens, token)
		}
	}
	g.mu.Unlock()
	for _, token := range tokens {
		g.remove(token)
	}
}

func (g *filePreviewGrants) revokeInstallation(installation string) {
	g.mu.Lock()
	var tokens []string
	for token, entry := range g.entries {
		if entry.scope.InstallationID == installation {
			tokens = append(tokens, token)
		}
	}
	g.mu.Unlock()
	for _, token := range tokens {
		g.remove(token)
	}
}

func (g *filePreviewGrants) reserve(user string) (func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := g.preparing[user]
	total := len(g.entries)
	for _, pending := range g.preparing {
		total += pending
	}
	for _, entry := range g.entries {
		if entry.scope.UserID == user {
			count++
		}
	}
	if g.closed || count >= 4 || total >= 64 {
		return nil, errFilePreviewPolicy
	}
	g.preparing[user]++
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.preparing[user]--
		if g.preparing[user] == 0 {
			delete(g.preparing, user)
		}
	}, nil
}
