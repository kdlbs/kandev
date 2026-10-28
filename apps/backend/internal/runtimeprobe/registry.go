// Package runtimeprobe holds the per-environment tokens of the worker-to-host
// reachability probe. A token is the only credential of the ping route, lives
// for at most TokenLifetime, and is deleted when its probe ends.
package runtimeprobe

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const (
	// PingPath is the route a worker requests to prove it can reach the host.
	PingPath = agentctltypes.HostAPIPingPath
	// HeaderName carries the probe token.
	HeaderName = agentctltypes.HostAPIProbeTokenHeader
	// TokenLifetime bounds a token that is never ended explicitly.
	TokenLifetime = 60 * time.Second
)

// Registry maps probe tokens to the environment they were issued for. Tokens
// are stored as digests so a lookup does not compare secret bytes.
type Registry struct {
	now     func() time.Time
	mu      sync.Mutex
	entries map[[sha256.Size]byte]*entry
}

type entry struct {
	environmentID string
	expires       time.Time
	reached       bool
}

// New returns an empty registry that reads the wall clock.
func New() *Registry { return newRegistry(time.Now) }

func newRegistry(now func() time.Time) *Registry {
	return &Registry{now: now, entries: map[[sha256.Size]byte]*entry{}}
}

// Probe is one environment's live token.
type Probe struct {
	registry *Registry
	key      [sha256.Size]byte
	token    string
}

// Begin issues a fresh random token for the environment.
func (r *Registry) Begin(environmentID string) *Probe {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("runtimeprobe: system random source is unavailable: " + err.Error())
	}
	token := hex.EncodeToString(raw)
	key := sha256.Sum256([]byte(token))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	r.entries[key] = &entry{environmentID: environmentID, expires: r.now().Add(TokenLifetime)}
	return &Probe{registry: r, key: key, token: token}
}

// Token is the value a worker sends in HeaderName.
func (p *Probe) Token() string { return p.token }

// Reached reports whether a request with this token matched, whether or not
// its reply arrived.
func (p *Probe) Reached() bool {
	p.registry.mu.Lock()
	defer p.registry.mu.Unlock()
	found, ok := p.registry.entries[p.key]
	return ok && found.reached
}

// End deletes the token. Calling it again is a no-op.
func (p *Probe) End() {
	p.registry.mu.Lock()
	defer p.registry.mu.Unlock()
	delete(p.registry.entries, p.key)
}

// Active is the number of live tokens.
func (r *Registry) Active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	return len(r.entries)
}

func (r *Registry) pruneLocked() {
	now := r.now()
	for key, found := range r.entries {
		if !now.Before(found.expires) {
			delete(r.entries, key)
		}
	}
}

func (r *Registry) match(token string) bool {
	if token == "" {
		return false
	}
	key := sha256.Sum256([]byte(token))
	r.mu.Lock()
	defer r.mu.Unlock()
	found, ok := r.entries[key]
	if !ok || !r.now().Before(found.expires) {
		delete(r.entries, key)
		return false
	}
	found.reached = true
	return true
}

// Handler answers 204 to a live token and 401 to anything else. It never
// reads the session or a PAT.
func (r *Registry) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.match(c.GetHeader(HeaderName)) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
