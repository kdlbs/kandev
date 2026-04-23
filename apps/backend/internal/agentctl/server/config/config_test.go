package config

import "testing"

func TestConsumeNonce(t *testing.T) {
	t.Run("valid nonce returns token and burns nonce", func(t *testing.T) {
		cfg := &Config{
			AuthToken:      "test-token-123",
			BootstrapNonce: "nonce-abc",
		}

		token := cfg.ConsumeNonce("nonce-abc")
		if token != "test-token-123" {
			t.Fatalf("expected test-token-123, got %q", token)
		}

		// Second call should fail (nonce burned)
		token2 := cfg.ConsumeNonce("nonce-abc")
		if token2 != "" {
			t.Fatalf("expected empty after nonce burn, got %q", token2)
		}
	})

	t.Run("wrong nonce returns empty", func(t *testing.T) {
		cfg := &Config{
			AuthToken:      "test-token",
			BootstrapNonce: "correct-nonce",
		}

		token := cfg.ConsumeNonce("wrong-nonce")
		if token != "" {
			t.Fatalf("expected empty for wrong nonce, got %q", token)
		}

		// Original nonce should still work
		token2 := cfg.ConsumeNonce("correct-nonce")
		if token2 != "test-token" {
			t.Fatalf("expected test-token, got %q", token2)
		}
	})

	t.Run("empty nonce returns empty", func(t *testing.T) {
		cfg := &Config{
			AuthToken:      "test-token",
			BootstrapNonce: "some-nonce",
		}

		token := cfg.ConsumeNonce("")
		if token != "" {
			t.Fatalf("expected empty for empty nonce, got %q", token)
		}
	})

	t.Run("no bootstrap nonce configured returns empty", func(t *testing.T) {
		cfg := &Config{
			AuthToken: "test-token",
		}

		token := cfg.ConsumeNonce("any-nonce")
		if token != "" {
			t.Fatalf("expected empty when no nonce configured, got %q", token)
		}
	})
}

func TestGenerateSelfToken(t *testing.T) {
	token := generateSelfToken()
	if len(token) != 64 { // 32 bytes hex-encoded = 64 chars
		t.Fatalf("expected 64-char hex token, got %d chars: %q", len(token), token)
	}

	// Tokens should be unique
	token2 := generateSelfToken()
	if token == token2 {
		t.Fatal("expected unique tokens, got identical")
	}
}
