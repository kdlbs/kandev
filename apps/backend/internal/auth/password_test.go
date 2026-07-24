package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected PHC prefix: %s", hash)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := VerifyPassword("wrong password", hash); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
}

func TestHashPasswordUniqueSalts(t *testing.T) {
	h1, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password must differ (unique salts)")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	for _, encoded := range []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA",  // wrong variant
		"$argon2id$v=18$m=65536,t=1,p=4$c2FsdA$aGFzaA", // wrong version
		"$argon2id$v=19$m=65536,t=1,p=4$!!!$aGFzaA",    // bad salt b64
	} {
		if err := VerifyPassword("x", encoded); err == nil {
			t.Fatalf("expected error for malformed hash %q", encoded)
		}
	}
}
