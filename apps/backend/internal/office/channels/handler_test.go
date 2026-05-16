package channels

import "testing"

func TestVerifyWebhookSignature_ValidHMAC(t *testing.T) {
	body := []byte("hello")
	sig := signWebhookBody("secret", body)

	if !verifyWebhookSignature("secret", "slack", body, sig) {
		t.Fatal("expected valid HMAC signature")
	}
}

func TestVerifyWebhookSignature_InvalidHMAC(t *testing.T) {
	if verifyWebhookSignature("secret", "slack", []byte("hello"), "sha256=bad") {
		t.Fatal("invalid signature should be rejected")
	}
}

func TestVerifyWebhookSignature_TelegramToken(t *testing.T) {
	if !verifyWebhookSignature("secret", "telegram", []byte("hello"), "secret") {
		t.Fatal("expected exact secret token to be accepted for Telegram")
	}
}

func TestVerifyWebhookSignature_TelegramRawRejectedForOtherPlatforms(t *testing.T) {
	// Raw secret token must NOT be accepted for non-Telegram platforms.
	if verifyWebhookSignature("secret", "slack", []byte("hello"), "secret") {
		t.Fatal("raw secret should be rejected for non-Telegram platforms")
	}
}
