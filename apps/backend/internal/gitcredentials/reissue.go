package gitcredentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	reissueCapabilityTTL = 30 * 24 * time.Hour
	reissueCapabilityAAD = "kandev.gitcredentials.reissue.v1"
)

type reissueCapabilityClaims struct {
	Scope     Scope `json:"scope"`
	ExpiresAt int64 `json:"expires_at"`
}

// ReissueCapabilitySigner encrypts scope claims. Its key must remain stable
// across backend restarts; callers must not log, persist, or expose that key.
type ReissueCapabilitySigner struct {
	key [sha256.Size]byte
}

func NewReissueCapabilitySigner(key string) (*ReissueCapabilitySigner, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrReissueUnavailable
	}
	return &ReissueCapabilitySigner{key: sha256.Sum256([]byte(reissueCapabilityAAD + ":" + key))}, nil
}

func (s *ReissueCapabilitySigner) Issue(scope Scope, now time.Time) (ReissueCapability, error) {
	if s == nil {
		return ReissueCapability{}, ErrReissueUnavailable
	}
	expiresAt := now.UTC().Add(reissueCapabilityTTL)
	payload, err := json.Marshal(reissueCapabilityClaims{Scope: scope, ExpiresAt: expiresAt.Unix()})
	if err != nil {
		return ReissueCapability{}, fmt.Errorf("encode Git credential reissue capability: %w", err)
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return ReissueCapability{}, fmt.Errorf("initialize Git credential reissue capability: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ReissueCapability{}, fmt.Errorf("initialize Git credential reissue capability: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ReissueCapability{}, fmt.Errorf("create Git credential reissue capability: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, payload, []byte(reissueCapabilityAAD))
	return ReissueCapability{Token: base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), ExpiresAt: expiresAt}, nil
}

func (s *ReissueCapabilitySigner) Validate(raw string, now time.Time) (Scope, error) {
	if s == nil || strings.TrimSpace(raw) == "" {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	encoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(encoded) < gcm.NonceSize() {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	payload, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], []byte(reissueCapabilityAAD))
	if err != nil {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	var claims reissueCapabilityClaims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.ExpiresAt == 0 {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	if !time.Unix(claims.ExpiresAt, 0).After(now) {
		return Scope{}, ErrReissueCapabilityExpired
	}
	scope, err := normalizeScope(claims.Scope)
	if err != nil {
		return Scope{}, ErrReissueCapabilityInvalid
	}
	return scope, nil
}
