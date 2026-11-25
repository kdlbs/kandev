package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
)

// TestHTTPSenderSpeaksWebPushToALocalPushService drives the production sender
// against a loopback stand-in for a push service, checking the RFC 8030/8291/8292
// wire shape without any external network.
func TestHTTPSenderSpeaksWebPushToALocalPushService(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	browserKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	_, _ = rand.Read(authSecret)
	sub := &models.PushSubscription{
		Endpoint: server.URL + "/push/abc",
		P256dh:   base64.RawURLEncoding.EncodeToString(browserKey.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(authSecret),
	}
	keys, err := generateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}

	status, err := newLocalHTTPSenderForTest().Send(context.Background(), sub, []byte(`{"title":"hi"}`), keys,
		"mailto:ops@example.com", SendOptions{TTL: 90 * time.Second, Urgency: UrgencyHigh, Topic: "topic1"})
	if err != nil || status != http.StatusCreated {
		t.Fatalf("Send() = %d, %v", status, err)
	}
	if got := gotHeaders.Get("TTL"); got != "90" {
		t.Errorf("TTL = %q, want 90", got)
	}
	if got := gotHeaders.Get("Urgency"); got != "high" {
		t.Errorf("Urgency = %q, want high", got)
	}
	if got := gotHeaders.Get("Topic"); got != "topic1" {
		t.Errorf("Topic = %q, want topic1", got)
	}
	if got := gotHeaders.Get("Content-Encoding"); got != "aes128gcm" {
		t.Errorf("Content-Encoding = %q, want aes128gcm", got)
	}
	if auth := gotHeaders.Get("Authorization"); !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, "k="+keys.PublicKey) {
		t.Errorf("Authorization = %q, want a VAPID header carrying our public key", auth)
	}
	if strings.Contains(string(gotBody), "title") {
		t.Error("body must be encrypted, found plaintext payload")
	}
}

func TestHTTPSenderRejectsUntrustedEndpoint(t *testing.T) {
	sender := NewHTTPSender()
	_, err := sender.Send(context.Background(), &models.PushSubscription{Endpoint: "https://127.0.0.1/push"}, nil,
		models.VAPIDKeys{}, "", SendOptions{})
	if err == nil {
		t.Fatal("Send() error = nil, want an untrusted endpoint to be rejected")
	}
}

func TestHTTPSenderDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer source.Close()

	browserKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}
	keys, err := generateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	status, err := newLocalHTTPSenderForTest().Send(context.Background(), &models.PushSubscription{
		Endpoint: source.URL,
		P256dh:   base64.RawURLEncoding.EncodeToString(browserKey.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(authSecret),
	}, []byte(`{"title":"hi"}`), keys, "mailto:ops@example.com", SendOptions{TTL: time.Minute})
	if err != nil || status != http.StatusFound {
		t.Fatalf("Send() = %d, %v; want redirect response returned without following", status, err)
	}
	if redirected.Load() {
		t.Fatal("HTTPSender followed a redirect to another destination")
	}
}

func newLocalHTTPSenderForTest() *HTTPSender {
	sender := NewHTTPSender()
	sender.allowUntrustedEndpoint = true
	return sender
}
