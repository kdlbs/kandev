package forgejo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestController_HandleWebhookAcceptsForgejoSignatureHeader(t *testing.T) {
	service, secrets := newConfigTestService(t)
	payload := []byte(`{}`)
	if err := secrets.Set(context.Background(), WebhookSecretKeyForWorkspace("ws"), "secret", "secret"); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(payload)
	router := gin.New()
	RegisterRoutes(router, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forgejo/webhooks?workspace_id=ws", strings.NewReader(string(payload)))
	request.Header.Set("X-Forgejo-Delivery", "delivery")
	request.Header.Set("X-Forgejo-Signature", hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
