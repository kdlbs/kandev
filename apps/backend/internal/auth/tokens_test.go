package auth

import (
	"strings"
	"testing"
)

func TestGenerateSessionToken(t *testing.T) {
	token, hash, err := GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || hash == "" {
		t.Fatal("empty token or hash")
	}
	if HashToken(token) != hash {
		t.Fatal("hash must be the digest of the token")
	}
	if IsPATFormat(token) {
		t.Fatal("session tokens must not look like PATs")
	}
	token2, _, err := GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == token2 {
		t.Fatal("tokens must be unique")
	}
}

func TestGeneratePAT(t *testing.T) {
	token, prefix, hash, err := GeneratePAT()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, PATPrefix+prefix+"_") {
		t.Fatalf("token %q must start with %q", token, PATPrefix+prefix+"_")
	}
	if len(prefix) != 8 {
		t.Fatalf("prefix length = %d, want 8", len(prefix))
	}
	if HashToken(token) != hash {
		t.Fatal("hash must be the digest of the full token")
	}
	if !IsPATFormat(token) {
		t.Fatal("IsPATFormat must recognize generated PATs")
	}
	if IsPATFormat("Bearer xyz") || IsPATFormat("eyJhbGciOi...") {
		t.Fatal("IsPATFormat must reject non-PAT bearers")
	}
}
