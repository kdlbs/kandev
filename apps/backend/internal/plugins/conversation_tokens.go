package plugins

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	conversationTokenTTL = 10 * time.Minute
	tokenKindBinding     = "binding"
	tokenKindCursor      = "cursor"
	tokenKindSnapshot    = "snapshot"
	tokenKindResume      = "resume"
)

// conversationGeneration remains exactly representable by JavaScript while
// retaining microsecond separation between plugin installations.
func conversationGeneration(installedAt time.Time) int64 {
	return installedAt.UnixMicro()
}

type conversationTokenClaims struct {
	Version     int      `json:"v"`
	Kind        string   `json:"kind"`
	PluginID    string   `json:"plugin_id"`
	UserID      string   `json:"user_id"`
	Generation  int64    `json:"generation"`
	ExpiresAt   int64    `json:"expires_at"`
	SessionID   string   `json:"session_id,omitempty"`
	TaskID      *string  `json:"task_id,omitempty"`
	Sort        string   `json:"sort,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	LastID      string   `json:"last_id,omitempty"`
	Cutoff      int64    `json:"cutoff,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	ConsumerID  string   `json:"consumer_id,omitempty"`
	WireID      string   `json:"wire_id,omitempty"`
	Sequence    uint64   `json:"sequence,omitempty"`
	Nonce       string   `json:"nonce"`
}

type conversationTokenManager struct {
	key []byte
	now func() time.Time
}

func newConversationTokenManager() *conversationTokenManager {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("plugins: cannot initialize conversation token signer: " + err.Error())
	}
	return &conversationTokenManager{key: key, now: time.Now}
}

func loadOrCreateConversationTokenManager(path string) (*conversationTokenManager, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		return conversationTokenManagerFromKey(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read conversation token signing key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create conversation token key directory: %w", err)
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate conversation token signing key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		key, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read concurrently created conversation token signing key: %w", err)
		}
		return conversationTokenManagerFromKey(key)
	}
	if err != nil {
		return nil, fmt.Errorf("create conversation token signing key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write conversation token signing key: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("sync conversation token signing key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close conversation token signing key: %w", err)
	}
	return conversationTokenManagerFromKey(key)
}

func conversationTokenManagerFromKey(key []byte) (*conversationTokenManager, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("conversation token signing key must be 32 bytes, got %d", len(key))
	}
	return &conversationTokenManager{key: append([]byte(nil), key...), now: time.Now}, nil
}

func (m *conversationTokenManager) mintBinding(
	pluginID string,
	userID string,
	generation int64,
) (string, time.Time, error) {
	expiresAt := m.now().UTC().Add(conversationTokenTTL)
	claims := conversationTokenClaims{
		Version: 1, Kind: tokenKindBinding, PluginID: pluginID, UserID: userID,
		Generation: generation, ExpiresAt: expiresAt.Unix(),
	}
	token, err := m.seal(claims)
	return token, expiresAt, err
}

func (m *conversationTokenManager) mintCursor(
	pluginID string,
	userID string,
	generation int64,
	sessionID string,
	taskID *string,
	sortOrder string,
	authors []string,
	lastID string,
	cutoff int64,
	fingerprint string,
) (string, error) {
	return m.seal(conversationTokenClaims{
		Version: 1, Kind: tokenKindCursor, PluginID: pluginID, UserID: userID,
		Generation: generation, ExpiresAt: m.now().UTC().Add(conversationTokenTTL).Unix(),
		SessionID: sessionID, TaskID: taskID, Sort: sortOrder, Authors: authors, LastID: lastID,
		Cutoff: cutoff, Fingerprint: fingerprint,
	})
}

func (m *conversationTokenManager) mintSnapshot(
	pluginID string,
	userID string,
	generation int64,
	sessionID string,
	taskID *string,
	sortOrder string,
	authors []string,
	cutoff int64,
	fingerprint string,
) (string, error) {
	return m.seal(conversationTokenClaims{
		Version: 1, Kind: tokenKindSnapshot, PluginID: pluginID, UserID: userID,
		Generation: generation, ExpiresAt: m.now().UTC().Add(conversationTokenTTL).Unix(),
		SessionID: sessionID, TaskID: taskID, Sort: sortOrder, Authors: authors,
		Cutoff: cutoff, Fingerprint: fingerprint,
	})
}

func (m *conversationTokenManager) renew(claims conversationTokenClaims) (string, time.Time, error) {
	expiresAt := m.now().UTC().Add(conversationTokenTTL)
	claims.ExpiresAt = expiresAt.Unix()
	claims.Nonce = ""
	token, err := m.seal(claims)
	return token, expiresAt, err
}

func (m *conversationTokenManager) mintResume(
	pluginID string,
	userID string,
	generation int64,
	sessionID string,
	consumerID string,
	wireID string,
	sequence uint64,
) (string, error) {
	return m.seal(conversationTokenClaims{
		Version: 1, Kind: tokenKindResume, PluginID: pluginID, UserID: userID,
		Generation: generation, ExpiresAt: m.now().UTC().Add(conversationTokenTTL).Unix(),
		SessionID: sessionID, ConsumerID: consumerID, WireID: wireID, Sequence: sequence,
	})
}

func (m *conversationTokenManager) seal(claims conversationTokenClaims) (string, error) {
	if claims.Nonce == "" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return "", err
		}
		claims.Nonce = base64.RawURLEncoding.EncodeToString(nonce)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

func (m *conversationTokenManager) parse(token string) (conversationTokenClaims, error) {
	var claims conversationTokenClaims
	payloadPart, signaturePart, ok := strings.Cut(token, ".")
	if !ok || payloadPart == "" || signaturePart == "" {
		return claims, errors.New("invalid token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return claims, errors.New("invalid token")
	}
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(payloadPart))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return claims, errors.New("invalid token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil || json.Unmarshal(payload, &claims) != nil || claims.Version != 1 {
		return conversationTokenClaims{}, errors.New("invalid token")
	}
	if !m.now().UTC().Before(time.Unix(claims.ExpiresAt, 0)) {
		return conversationTokenClaims{}, errors.New("expired token")
	}
	return claims, nil
}
