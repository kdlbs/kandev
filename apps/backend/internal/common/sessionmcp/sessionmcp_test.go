package sessionmcp

import (
	"strings"
	"testing"
)

func TestTokenIsDeterministicPerKeyInstanceAndNonce(t *testing.T) {
	key := []byte(strings.Repeat("k", KeyLength))
	a := Token(key, "inst-1", "aa")
	if a == "" || a != Token(key, "inst-1", "aa") {
		t.Fatalf("token not deterministic: %q", a)
	}
	for name, other := range map[string]string{
		"instance": Token(key, "inst-2", "aa"),
		"nonce":    Token(key, "inst-1", "bb"),
		"key":      Token([]byte(strings.Repeat("j", KeyLength)), "inst-1", "aa"),
	} {
		if other == a {
			t.Fatalf("token did not change with %s", name)
		}
	}
}

func TestTokenRejectsWrongKeyLength(t *testing.T) {
	if Token([]byte("short"), "i", "n") != "" {
		t.Fatal("expected empty token for a short key")
	}
}

func TestNonceIsFreshHex(t *testing.T) {
	a, err := NewNonce()
	if err != nil || len(a) != 32 {
		t.Fatalf("nonce %q err %v", a, err)
	}
	if b, _ := NewNonce(); a == b {
		t.Fatal("nonce repeated")
	}
}

func TestFingerprintsAreDomainSeparatedAndEmptyForEmpty(t *testing.T) {
	if Fingerprint("") != "" || BrokerFingerprint("") != "" {
		t.Fatal("empty input must fingerprint to empty")
	}
	if Fingerprint("x") == BrokerFingerprint("x") || Fingerprint("x") == "x" {
		t.Fatal("fingerprints must be domain separated")
	}
}
