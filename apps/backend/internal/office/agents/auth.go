package agents

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AgentClaims holds the claims in an agent JWT.
type AgentClaims struct {
	AgentProfileID string `json:"agent_profile_id"`
	TaskID         string `json:"task_id"`
	WorkspaceID    string `json:"workspace_id"`
	RunID          string `json:"run_id,omitempty"`
	SessionID      string `json:"session_id"`
	Capabilities   string `json:"capabilities,omitempty"`
	ExpiresAt      int64  `json:"exp"`
	IssuedAt       int64  `json:"iat"`
}

// JWT errors.
var (
	ErrTokenMalformed = errors.New("malformed token")
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenSignature = errors.New("invalid signature")
)

// DefaultTokenDuration is the default validity period for agent JWTs.
const DefaultTokenDuration = 4 * time.Hour

// AgentAuth provides JWT minting and validation for agent API access.
type AgentAuth struct {
	signingKey []byte
}

// NewAgentAuth creates an AgentAuth with the given signing key.
// If key is empty, a random 32-byte key is generated.
func NewAgentAuth(key string) *AgentAuth {
	var signingKey []byte
	if key != "" {
		signingKey = []byte(key)
	} else {
		signingKey = make([]byte, 32)
		if _, err := rand.Read(signingKey); err != nil {
			panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
		}
	}
	return &AgentAuth{signingKey: signingKey}
}

// MintAgentJWT creates a signed JWT for an agent session.
func (a *AgentAuth) MintAgentJWT(
	agentInstanceID, taskID, workspaceID, sessionID string,
) (string, error) {
	return a.mintRuntimeWithExpiry(agentInstanceID, taskID, workspaceID, "", sessionID, "", DefaultTokenDuration)
}

// MintRuntimeJWT creates a signed JWT with run-scoped capabilities.
func (a *AgentAuth) MintRuntimeJWT(
	agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities string,
) (string, error) {
	return a.mintRuntimeWithExpiry(
		agentInstanceID,
		taskID,
		workspaceID,
		runID,
		sessionID,
		capabilities,
		DefaultTokenDuration,
	)
}

func (a *AgentAuth) mintWithExpiry(
	agentInstanceID, taskID, workspaceID, sessionID string,
	duration time.Duration,
) (string, error) {
	return a.mintRuntimeWithExpiry(agentInstanceID, taskID, workspaceID, "", sessionID, "", duration)
}

func (a *AgentAuth) mintRuntimeWithExpiry(
	agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities string,
	duration time.Duration,
) (string, error) {
	now := time.Now()
	claims := AgentClaims{
		AgentProfileID: agentInstanceID,
		TaskID:         taskID,
		WorkspaceID:    workspaceID,
		RunID:          runID,
		SessionID:      sessionID,
		Capabilities:   capabilities,
		ExpiresAt:      now.Add(duration).Unix(),
		IssuedAt:       now.Unix(),
	}
	return signJWT(a.signingKey, &claims)
}

// MintExpiredJWT creates a JWT that is already expired. Useful for testing.
func (a *AgentAuth) MintExpiredJWT(
	agentInstanceID, taskID, workspaceID, sessionID string,
) (string, error) {
	return a.mintWithExpiry(agentInstanceID, taskID, workspaceID, sessionID, -1*time.Hour)
}

// ValidateAgentJWT parses and validates a signed JWT.
func (a *AgentAuth) ValidateAgentJWT(tokenString string) (*AgentClaims, error) {
	claims, err := verifyJWT(a.signingKey, tokenString)
	if err != nil {
		return nil, err
	}
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}
	return claims, nil
}

// signJWT creates a HS256-signed JWT from claims.
func signJWT(key []byte, claims *AgentClaims) (string, error) {
	header := base64Encode([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadB64 := base64Encode(payload)
	sigInput := header + "." + payloadB64
	sig := hmacSHA256(key, []byte(sigInput))
	return sigInput + "." + base64Encode(sig), nil
}

// verifyJWT verifies the signature and decodes claims.
func verifyJWT(key []byte, token string) (*AgentClaims, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, ErrTokenMalformed
	}
	sigInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256(key, []byte(sigInput))
	actualSig, err := base64Decode(parts[2])
	if err != nil {
		return nil, ErrTokenMalformed
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrTokenSignature
	}
	payloadBytes, err := base64Decode(parts[1])
	if err != nil {
		return nil, ErrTokenMalformed
	}
	var claims AgentClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrTokenMalformed
	}
	return &claims, nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func base64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// SetAuth sets the JWT auth on the service so it can validate tokens.
func (s *AgentService) SetAuth(auth *AgentAuth) {
	s.auth = auth
}

// ValidateAgentJWT validates an agent JWT and returns the claims.
// Returns an error if no AgentAuth has been configured.
func (s *AgentService) ValidateAgentJWT(token string) (*AgentClaims, error) {
	if s.auth == nil {
		return nil, fmt.Errorf("agent auth not configured")
	}
	return s.auth.ValidateAgentJWT(token)
}

// MintRuntimeJWT creates a per-run token for runtime syscall routes.
func (s *AgentService) MintRuntimeJWT(agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities string) (string, error) {
	if s.auth == nil {
		return "", fmt.Errorf("agent auth not configured")
	}
	return s.auth.MintRuntimeJWT(agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities)
}
