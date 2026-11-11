package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWebhookValidate(t *testing.T) {
	provider := NewWebhookProvider(false)
	const whitespacePaddedHeaderName = " X-Tenant "
	cases := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{name: "valid public ip", config: map[string]interface{}{"url": "https://93.184.216.34/hooks/kandev"}},
		{name: "valid public hostname", config: map[string]interface{}{"url": "https://hooks.example.com/kandev"}},
		{name: "valid with headers and secret", config: map[string]interface{}{
			"url":     "http://93.184.216.34/hooks/kandev",
			"headers": map[string]interface{}{"X-Tenant": "acme"},
			"secret":  "token",
		}},
		{name: "missing config", config: nil, wantErr: true},
		{name: "missing url", config: map[string]interface{}{}, wantErr: true},
		{name: "blank url", config: map[string]interface{}{"url": "  "}, wantErr: true},
		{name: "invalid url", config: map[string]interface{}{"url": "://not-a-url"}, wantErr: true},
		{name: "unsupported scheme", config: map[string]interface{}{"url": "ftp://example.com"}, wantErr: true},
		{name: "missing host", config: map[string]interface{}{"url": "http://"}, wantErr: true},
		{name: "non string url", config: map[string]interface{}{"url": 42}, wantErr: true},
		{name: "loopback ipv4", config: map[string]interface{}{"url": "http://127.0.0.1/hook"}, wantErr: true},
		{name: "loopback ipv6", config: map[string]interface{}{"url": "http://[::1]/hook"}, wantErr: true},
		{name: "loopback ipv4 mapped ipv6", config: map[string]interface{}{"url": "http://[::ffff:127.0.0.1]/hook"}, wantErr: true},
		{name: "loopback trailing dot", config: map[string]interface{}{"url": "http://127.0.0.1./hook"}, wantErr: true},
		{name: "private ipv4", config: map[string]interface{}{"url": "http://10.1.2.3/hook"}, wantErr: true},
		{name: "private ipv4 mapped ipv6", config: map[string]interface{}{"url": "http://[::ffff:10.1.2.3]/hook"}, wantErr: true},
		{name: "private ipv6", config: map[string]interface{}{"url": "http://[fd00::1]/hook"}, wantErr: true},
		{name: "link local metadata", config: map[string]interface{}{"url": "http://169.254.169.254/latest/meta-data"}, wantErr: true},
		{name: "link local ipv6", config: map[string]interface{}{"url": "http://[fe80::1]/hook"}, wantErr: true},
		{name: "unspecified ipv4", config: map[string]interface{}{"url": "http://0.0.0.0/hook"}, wantErr: true},
		{name: "unspecified ipv6", config: map[string]interface{}{"url": "http://[::]/hook"}, wantErr: true},
		{name: "reserved documentation address", config: map[string]interface{}{"url": "http://192.0.2.10/hook"}, wantErr: true},
		{name: "localhost hostname", config: map[string]interface{}{"url": "http://localhost:8080/hook"}, wantErr: true},
		{name: "localhost trailing dot", config: map[string]interface{}{"url": "http://localhost./hook"}, wantErr: true},
		{name: "non object headers", config: map[string]interface{}{"url": "https://93.184.216.34", "headers": "X-Test: value"}, wantErr: true},
		{name: "non string header value", config: map[string]interface{}{"url": "https://93.184.216.34", "headers": map[string]interface{}{"X-Test": 1}}, wantErr: true},
		{name: "invalid header name", config: map[string]interface{}{"url": "https://93.184.216.34", "headers": map[string]interface{}{"X Tenant": "acme"}}, wantErr: true},
		{name: "invalid header value", config: map[string]interface{}{"url": "https://93.184.216.34", "headers": map[string]interface{}{"X-Tenant": "bad\nvalue"}}, wantErr: true},
		{name: "duplicate header names", config: map[string]interface{}{"url": "https://93.184.216.34", "headers": map[string]interface{}{"X-Tenant": "acme", "x-tenant": "other"}}, wantErr: true},
		{name: "duplicate header names with whitespace", config: map[string]interface{}{"url": "https://93.184.216.34", "headers": map[string]interface{}{whitespacePaddedHeaderName: "acme", "x-tenant": "other"}}, wantErr: true},
		{name: "non string secret", config: map[string]interface{}{"url": "https://93.184.216.34", "secret": 1}, wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := provider.Validate(testCase.config)
			if testCase.wantErr && err == nil {
				t.Fatalf("expected validation error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestWebhookValidateAllowsPrivateDestinationWhenOptedIn(t *testing.T) {
	provider := NewWebhookProvider(true)
	for _, rawURL := range []string{
		"http://127.0.0.1/hook",
		"http://10.1.2.3/hook",
		"http://169.254.169.254/latest/meta-data",
		"http://[fd00::1]/hook",
	} {
		if err := provider.Validate(map[string]interface{}{"url": rawURL}); err != nil {
			t.Fatalf("validate %s: %v", rawURL, err)
		}
	}
}

func TestWebhookValidateRejectsIPv6TransitionDestinations(t *testing.T) {
	provider := NewWebhookProvider(false)
	for _, rawURL := range []string{
		"http://[64:ff9b::a00:1]/hook",
		"http://[2002:a00:1::1]/hook",
	} {
		if err := provider.Validate(map[string]interface{}{"url": rawURL}); err == nil {
			t.Errorf("validate %s: expected embedded private IPv4 destination to be blocked", rawURL)
		}
	}
}

func TestWebhookSendRejectsPrivateDestinationAtDialTime(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(false)
	err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config:    map[string]interface{}{"url": server.URL},
	})
	if err == nil {
		t.Fatal("expected private destination rejection")
	}
	if !strings.Contains(err.Error(), "globally routable") {
		t.Fatalf("error = %q", err)
	}
	if hits != 0 {
		t.Fatalf("server received %d requests, want 0", hits)
	}
}

func TestWebhookSendPayloadHeadersAndSecret(t *testing.T) {
	type captured struct {
		headers http.Header
		body    webhookPayload
	}
	received := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		received <- captured{headers: r.Header.Clone(), body: payload}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := NewWebhookProvider(true)
	err := provider.Send(context.Background(), Message{
		EventType:     "session.turn_finished",
		Title:         "Agent turn finished",
		Body:          "The agent finished a turn.",
		TaskID:        "task-1",
		TaskSessionID: "session-1",
		OccurrenceID:  "occurrence-1",
		UserID:        "user-1",
		Config: map[string]interface{}{
			"url":     server.URL,
			"headers": map[string]interface{}{"X-Tenant": "acme"},
			"secret":  "gateway-token",
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	result := <-received
	if result.headers.Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", result.headers.Get("Content-Type"))
	}
	if result.headers.Get("X-Tenant") != "acme" {
		t.Fatalf("custom header = %q", result.headers.Get("X-Tenant"))
	}
	if result.headers.Get("Authorization") != "Bearer gateway-token" {
		t.Fatalf("authorization = %q", result.headers.Get("Authorization"))
	}
	if result.body.Version != "1.0" || result.body.Event != "session.turn_finished" ||
		result.body.Title != "Agent turn finished" || result.body.Message != "The agent finished a turn." ||
		result.body.Type != "info" {
		t.Fatalf("unexpected payload: %+v", result.body)
	}
	wantFields := map[string]string{
		"task_id":         "task-1",
		"task_session_id": "session-1",
		"occurrence_id":   "occurrence-1",
	}
	if len(result.body.Fields) != len(wantFields) {
		t.Fatalf("fields = %+v", result.body.Fields)
	}
	for key, value := range wantFields {
		if result.body.Fields[key] != value {
			t.Fatalf("field %s = %q, want %q", key, result.body.Fields[key], value)
		}
	}
	if _, ok := result.body.Fields["user_id"]; ok {
		t.Fatalf("payload must not include user identifiers")
	}
}

func TestWebhookSendOmitsEmptyFields(t *testing.T) {
	received := make(chan webhookPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		received <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(true)
	if err := provider.Send(context.Background(), Message{
		EventType: "session.clarification_requested",
		Title:     "Test notification",
		Body:      "test",
		Config:    map[string]interface{}{"url": server.URL},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := <-received
	if len(payload.Fields) != 0 {
		t.Fatalf("fields = %+v, want empty", payload.Fields)
	}
}

func TestWebhookSendHonorsConfiguredAuthorizationHeader(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(true)
	if err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config: map[string]interface{}{
			"url":     server.URL,
			"headers": map[string]interface{}{"authorization": "Token explicit"},
			"secret":  "ignored-secret",
		},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := (<-received).Get("Authorization"); got != "Token explicit" {
		t.Fatalf("authorization = %q", got)
	}
}

func TestWebhookSendNon2xxReturnsStatusWithoutBody(t *testing.T) {
	longBody := strings.Repeat("x", webhookMaxResponseBody*2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(longBody))
	}))
	defer server.Close()

	provider := NewWebhookProvider(true)
	err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config:    map[string]interface{}{"url": server.URL},
	})
	if err == nil {
		t.Fatal("expected non-2xx error")
	}
	message := err.Error()
	if !strings.Contains(message, "502") {
		t.Fatalf("error = %q", message)
	}
	if message != "webhook returned status 502" {
		t.Fatalf("error = %q, want only the status code", message)
	}
}

func TestWebhookSendDoesNotLeakCredentialsInTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	requestURL := server.URL + "/hook/path-credential?token=query-credential"
	server.Close()

	err := NewWebhookProvider(true).Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config:    map[string]interface{}{"url": requestURL},
	})
	if err == nil {
		t.Fatal("expected transport error")
	}
	for _, credential := range []string{"path-credential", "query-credential", requestURL} {
		if strings.Contains(err.Error(), credential) {
			t.Fatalf("transport error %q contains credential-bearing URL part %q", err, credential)
		}
	}
}

func TestWebhookSendDoesNotLeakResponseBodies(t *testing.T) {
	const echoedCredential = "echoed-webhook-credential"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	defer server.Close()

	err := NewWebhookProvider(true).Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config: map[string]interface{}{
			"url":    server.URL,
			"secret": echoedCredential,
		},
	})
	if err == nil {
		t.Fatal("expected non-2xx error")
	}
	if strings.Contains(err.Error(), echoedCredential) {
		t.Fatalf("webhook error %q contains response body", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("webhook error = %q, want response status", err)
	}
}

func TestWebhookSendTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(true)
	provider.client = &http.Client{Timeout: 20 * time.Millisecond}
	if err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config:    map[string]interface{}{"url": server.URL},
	}); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWebhookSendRejectsCrossOriginRedirect(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		if r.Header.Get("X-API-Key") != "" {
			t.Errorf("credential header reached the redirect target")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	provider := NewWebhookProvider(true)
	err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config: map[string]interface{}{
			"url":     redirect.URL,
			"headers": map[string]interface{}{"X-API-Key": "secret-value"},
		},
	})
	if err == nil {
		t.Fatal("expected cross-origin redirect rejection")
	}
	if err.Error() != "webhook request failed" {
		t.Fatalf("error = %q, want a sanitized request error", err)
	}
	if targetHits != 0 {
		t.Fatalf("redirect target received %d requests, want 0", targetHits)
	}
}

func TestWebhookSendFollowsSameOriginRedirect(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
			return
		}
		received <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(true)
	if err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config: map[string]interface{}{
			"url":     server.URL + "/redirect",
			"headers": map[string]interface{}{"X-API-Key": "secret-value"},
		},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := (<-received).Get("X-API-Key"); got != "secret-value" {
		t.Fatalf("api key = %q", got)
	}
}

func TestWebhookSendRejectsInvalidConfig(t *testing.T) {
	provider := NewWebhookProvider(false)
	if err := provider.Send(context.Background(), Message{}); err == nil {
		t.Fatal("expected invalid config error")
	}
}

func TestWebhookValidateAcceptsStringMapHeaders(t *testing.T) {
	provider := NewWebhookProvider(false)
	if err := provider.Validate(map[string]interface{}{
		"url":     "https://93.184.216.34",
		"headers": map[string]string{"X-Tenant": "acme"},
	}); err != nil {
		t.Fatalf("valid string map headers: %v", err)
	}
	if err := provider.Validate(map[string]interface{}{
		"url":     "https://93.184.216.34",
		"headers": map[string]string{"X-Tenant": "acme", "x-tenant": "other"},
	}); err == nil {
		t.Fatal("expected case-insensitive duplicate rejection")
	}
}

func TestWebhookDialContextRejectsLoopbackHostname(t *testing.T) {
	_, err := webhookDialContext(context.Background(), "tcp", "localhost:80", false)
	if !errors.Is(err, errWebhookDestinationBlocked) {
		t.Fatalf("err = %v, want destination blocked", err)
	}
}

func TestWebhookSendRejectsCrossPortRedirect(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	provider := NewWebhookProvider(true)
	err := provider.Send(context.Background(), Message{
		EventType: "session.turn_finished",
		Config:    map[string]interface{}{"url": redirect.URL},
	})
	if err == nil {
		t.Fatal("expected cross-port redirect rejection")
	}
	if targetHits != 0 {
		t.Fatalf("redirect target received %d requests, want 0", targetHits)
	}
}

func TestSameWebhookOrigin(t *testing.T) {
	cases := []struct {
		name  string
		first string
		next  string
		want  bool
	}{
		{name: "same implicit origin", first: "https://example.com/a", next: "https://example.com/b", want: true},
		{name: "explicit default https port", first: "https://example.com/a", next: "https://example.com:443/b", want: true},
		{name: "explicit default http port", first: "http://example.com/a", next: "http://example.com:80/b", want: true},
		{name: "different port", first: "https://example.com/a", next: "https://example.com:8443/b"},
		{name: "different scheme", first: "http://example.com/a", next: "https://example.com/b"},
		{name: "different host", first: "https://example.com/a", next: "https://other.example.com/b"},
		{name: "host case", first: "https://Example.com/a", next: "https://example.COM/b", want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			first, err := url.Parse(testCase.first)
			if err != nil {
				t.Fatal(err)
			}
			next, err := url.Parse(testCase.next)
			if err != nil {
				t.Fatal(err)
			}
			if got := sameWebhookOrigin(first, next); got != testCase.want {
				t.Fatalf("sameWebhookOrigin(%q, %q) = %v, want %v", testCase.first, testCase.next, got, testCase.want)
			}
		})
	}
}
