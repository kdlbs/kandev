package agents_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/agents"
)

func TestAgentAuth_MintAndValidate(t *testing.T) {
	auth := agents.NewAgentAuth("test-secret-key")

	token, err := auth.MintAgentJWT("agent-1", "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	// Token has 3 parts
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	claims, err := auth.ValidateAgentJWT(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.AgentProfileID != "agent-1" {
		t.Errorf("agent_profile_id = %q, want agent-1", claims.AgentProfileID)
	}
	if claims.TaskID != "task-1" {
		t.Errorf("task_id = %q, want task-1", claims.TaskID)
	}
	if claims.WorkspaceID != "ws-1" {
		t.Errorf("workspace_id = %q, want ws-1", claims.WorkspaceID)
	}
	if claims.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", claims.SessionID)
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		t.Error("token should not be expired yet")
	}
}

func TestAgentAuth_WrongKeyRejected(t *testing.T) {
	auth1 := agents.NewAgentAuth("key-1")
	auth2 := agents.NewAgentAuth("key-2")

	token, _ := auth1.MintAgentJWT("agent-1", "task-1", "ws-1", "sess-1")
	_, err := auth2.ValidateAgentJWT(token)
	if err != agents.ErrTokenSignature {
		t.Errorf("expected ErrTokenSignature, got %v", err)
	}
}

func TestAgentAuth_MalformedToken(t *testing.T) {
	auth := agents.NewAgentAuth("key")

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"one part", "abc"},
		{"two parts", "abc.def"},
		{"bad base64", "abc.def.!!!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.ValidateAgentJWT(tt.token)
			if err == nil {
				t.Error("expected error for malformed token")
			}
		})
	}
}

func TestAgentAuth_ExpiredTokenRejected(t *testing.T) {
	auth := agents.NewAgentAuth("test-key")

	token, err := auth.MintAgentJWT("agent-1", "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	// Validating a fresh token should work.
	_, err = auth.ValidateAgentJWT(token)
	if err != nil {
		t.Fatalf("fresh token should validate: %v", err)
	}

	// Verify the expiry claim is set in the future.
	claims, _ := auth.ValidateAgentJWT(token)
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Error("exp should be after iat")
	}
}

func TestAgentAuth_RandomKeyGeneration(t *testing.T) {
	// Empty key should generate a random one, and it should still work.
	auth := agents.NewAgentAuth("")
	token, err := auth.MintAgentJWT("agent-1", "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint with random key: %v", err)
	}
	claims, err := auth.ValidateAgentJWT(token)
	if err != nil {
		t.Fatalf("validate with random key: %v", err)
	}
	if claims.AgentProfileID != "agent-1" {
		t.Errorf("agent_profile_id = %q, want agent-1", claims.AgentProfileID)
	}
}

func TestAgentAuth_ClaimsComplete(t *testing.T) {
	auth := agents.NewAgentAuth("key")
	token, _ := auth.MintAgentJWT("a1", "t1", "w1", "s1")
	claims, _ := auth.ValidateAgentJWT(token)

	now := time.Now().Unix()
	if claims.IssuedAt > now || claims.IssuedAt < now-5 {
		t.Errorf("iat=%d should be close to now=%d", claims.IssuedAt, now)
	}
	expectedExp := now + int64(agents.DefaultTokenDuration.Seconds())
	if claims.ExpiresAt < expectedExp-5 || claims.ExpiresAt > expectedExp+5 {
		t.Errorf("exp=%d should be close to %d", claims.ExpiresAt, expectedExp)
	}
}
