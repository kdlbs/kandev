package webapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const RuntimeTokenTTL = 15 * time.Minute

const maxExpiredCapabilityTombstones = 1024

var (
	ErrRuntimeTokenInvalid = errors.New("runtime_token_invalid")
	ErrRuntimeTokenExpired = errors.New("runtime_token_expired")
	ErrRuntimeTokenStale   = errors.New("runtime_token_stale")
)

// CapabilityBinding is the complete authority binding for one runtime URL.
// It contains identifiers and a release artifact reference, never a user
// credential or a source payload.
type CapabilityBinding struct {
	PluginID        string
	UserID          string
	InstanceID      string
	ReleaseID       string
	WebAppKey       string
	Placement       string
	ScopeKind       string
	WorkspaceID     string
	TaskID          string
	SessionID       string
	RepositoryID    string
	GrantGeneration int64
	Permissions     []string
	Artifact        Artifact
	Entry           string
	NetworkOrigins  []string
}

type storedCapability struct {
	binding   CapabilityBinding
	issuedAt  time.Time
	expiresAt time.Time
}

// TokenManager stores only a SHA-256 digest of each random runtime token.
// Tokens are deliberately process-local and short-lived; instance and grant
// changes are checked by the caller through BindingValidator on every request.
type TokenManager struct {
	mu      sync.Mutex
	tokens  map[[sha256.Size]byte]storedCapability
	expired map[[sha256.Size]byte]time.Time
	now     func() time.Time
}

func NewTokenManager(now func() time.Time) *TokenManager {
	if now == nil {
		now = time.Now
	}
	return &TokenManager{tokens: make(map[[sha256.Size]byte]storedCapability), expired: make(map[[sha256.Size]byte]time.Time), now: now}
}

func (m *TokenManager) Issue(binding CapabilityBinding, ttl time.Duration) (string, error) {
	if m == nil || binding.UserID == "" || binding.InstanceID == "" || binding.ReleaseID == "" || binding.WebAppKey == "" || binding.Entry == "" {
		return "", ErrRuntimeTokenInvalid
	}
	if ttl <= 0 || ttl > RuntimeTokenTTL {
		ttl = RuntimeTokenTTL
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate runtime capability: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	issuedAt := m.now().UTC()
	m.mu.Lock()
	m.sweepExpiredLocked(issuedAt)
	digest := digestToken(token)
	delete(m.expired, digest)
	m.tokens[digest] = storedCapability{binding: binding, issuedAt: issuedAt, expiresAt: issuedAt.Add(ttl)}
	m.mu.Unlock()
	return token, nil
}

func (m *TokenManager) IssueURL(baseURL string, binding CapabilityBinding, ttl time.Duration) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrRuntimeTokenInvalid
	}
	token, err := m.Issue(binding, ttl)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/plugins/web-apps/runtime/" + token + "/"
	return parsed.String(), nil
}

func (m *TokenManager) Validate(token string) (CapabilityBinding, error) {
	if m == nil || strings.TrimSpace(token) == "" {
		return CapabilityBinding{}, ErrRuntimeTokenInvalid
	}
	digest := digestToken(token)
	now := m.now().UTC()
	m.mu.Lock()
	m.sweepExpiredLocked(now)
	capability, ok := m.tokens[digest]
	if ok && !now.Before(capability.expiresAt) {
		delete(m.tokens, digest)
		m.expired[digest] = now
		ok = false
	}
	_, wasExpired := m.expired[digest]
	m.mu.Unlock()
	if !ok {
		if wasExpired {
			return CapabilityBinding{}, ErrRuntimeTokenExpired
		}
		return CapabilityBinding{}, ErrRuntimeTokenInvalid
	}
	return capability.binding, nil
}

// Renew extends a valid token by at most RuntimeTokenTTL. The raw token is
// never copied into the stored capability.
func (m *TokenManager) Renew(token string, ttl time.Duration) error {
	if m == nil || strings.TrimSpace(token) == "" {
		return ErrRuntimeTokenInvalid
	}
	if ttl <= 0 || ttl > RuntimeTokenTTL {
		ttl = RuntimeTokenTTL
	}
	digest := digestToken(token)
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(now)
	capability, ok := m.tokens[digest]
	if !ok {
		if _, wasExpired := m.expired[digest]; wasExpired {
			return ErrRuntimeTokenExpired
		}
		return ErrRuntimeTokenInvalid
	}
	if !now.Before(capability.expiresAt) {
		delete(m.tokens, digest)
		m.expired[digest] = now
		return ErrRuntimeTokenExpired
	}
	capability.expiresAt = now.Add(ttl)
	m.tokens[digest] = capability
	return nil
}

func (m *TokenManager) Revoke(token string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.tokens, digestToken(token))
	delete(m.expired, digestToken(token))
	m.mu.Unlock()
}

func (m *TokenManager) StoredTokenCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(m.now().UTC())
	return len(m.tokens)
}

func (m *TokenManager) sweepExpiredLocked(now time.Time) {
	cutoff := now.Add(-RuntimeTokenTTL)
	for digest, expiredAt := range m.expired {
		if !expiredAt.After(cutoff) {
			delete(m.expired, digest)
		}
	}
	for len(m.expired) > maxExpiredCapabilityTombstones {
		var oldestDigest [sha256.Size]byte
		var oldestAt time.Time
		for digest, expiredAt := range m.expired {
			if oldestAt.IsZero() || expiredAt.Before(oldestAt) {
				oldestDigest, oldestAt = digest, expiredAt
			}
		}
		delete(m.expired, oldestDigest)
	}
}

// HasRawToken is a diagnostic test helper. Since the map key is a fixed-size
// digest, this is always false unless a future implementation regresses to a
// raw-token map.
func (m *TokenManager) HasRawToken(token string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := any(m.tokens).(map[string]storedCapability)
	return ok && token != ""
}

func digestToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// BindingValidator rechecks instance status, active release, grants, scope,
// and current caller authorization on every runtime request.
type BindingValidator func(context.Context, CapabilityBinding) error
