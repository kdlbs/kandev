package forgejo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyWebhookSignatureAndReplayProtection(t *testing.T) {
	payload := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if err := VerifyWebhookSignature("secret", payload, signature); err != nil {
		t.Fatal(err)
	}
	if err := VerifyWebhookSignature("wrong", payload, signature); err != ErrInvalidWebhookSignature {
		t.Fatalf("err=%v", err)
	}
	store := newConfigTestStore(t)
	first, err := store.RecordWebhookDelivery(context.Background(), "workspace-a", "delivery-1", payload)
	if err != nil || !first {
		t.Fatalf("first=%t err=%v", first, err)
	}
	second, err := store.RecordWebhookDelivery(context.Background(), "workspace-a", "delivery-1", payload)
	if err != nil || second {
		t.Fatalf("second=%t err=%v", second, err)
	}
}
