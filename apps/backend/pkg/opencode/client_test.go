package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestGenerateServerPassword(t *testing.T) {
	// Generate multiple passwords and ensure they're unique
	passwords := make(map[string]bool)
	for i := 0; i < 10; i++ {
		pw := GenerateServerPassword()
		if pw == "" {
			t.Error("generated empty password")
		}
		if passwords[pw] {
			t.Error("generated duplicate password")
		}
		passwords[pw] = true
	}
}

func TestClient_BuildAuthHeader(t *testing.T) {
	client := NewClient("http://localhost:8080", "/workspace", "test-password", newTestLogger())

	header := client.buildAuthHeader()
	if !strings.HasPrefix(header, "Basic ") {
		t.Errorf("expected header to start with 'Basic ', got %s", header)
	}
}

func TestClient_WaitForHealth(t *testing.T) {
	tests := []struct {
		name      string
		responses []HealthResponse
		delays    []time.Duration
		wantError bool
	}{
		{
			name:      "healthy immediately",
			responses: []HealthResponse{{Healthy: true, Version: "1.0.0"}},
			delays:    []time.Duration{0},
			wantError: false,
		},
		{
			name: "healthy after retry",
			responses: []HealthResponse{
				{Healthy: false, Version: "1.0.0"},
				{Healthy: true, Version: "1.0.0"},
			},
			delays:    []time.Duration{0, 0},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/global/health") {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}

				if callCount < len(tt.delays) {
					time.Sleep(tt.delays[callCount])
				}

				resp := tt.responses[callCount%len(tt.responses)]
				callCount++

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := client.WaitForHealth(ctx)
			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestClient_CreateSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.Contains(r.URL.Path, "/session") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Check auth header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Basic ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(SessionResponse{ID: "sess-123"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
	ctx := context.Background()

	sessionID, err := client.CreateSession(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID != "sess-123" {
		t.Errorf("expected session ID 'sess-123', got %s", sessionID)
	}
}

func TestClient_ForkSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.Contains(r.URL.Path, "/session/sess-123/fork") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(SessionResponse{ID: "sess-456"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
	ctx := context.Background()

	newSessionID, err := client.ForkSession(ctx, "sess-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newSessionID != "sess-456" {
		t.Errorf("expected session ID 'sess-456', got %s", newSessionID)
	}
}

func TestClient_SendPrompt(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantError  bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			response:   `{"info":{},"parts":[]}`,
			wantError:  false,
		},
		{
			name:       "error response",
			statusCode: http.StatusOK,
			response:   `{"name":"SomeError","data":{"message":"something went wrong"}}`,
			wantError:  true,
		},
		{
			name:       "http error",
			statusCode: http.StatusInternalServerError,
			response:   `{"error":"internal error"}`,
			wantError:  true,
		},
		{
			name:       "empty response",
			statusCode: http.StatusOK,
			response:   ``,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = fmt.Fprint(w, tt.response)
			}))
			defer server.Close()

			client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
			ctx := context.Background()

			err := client.SendPrompt(ctx, "sess-123", "Hello", nil, "", "")
			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestClient_SendPrompt_WithModel(t *testing.T) {
	var receivedBody PromptRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"info":{},"parts":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
	ctx := context.Background()

	model := &ModelSpec{ProviderID: "anthropic", ModelID: "claude-3-sonnet"}
	err := client.SendPrompt(ctx, "sess-123", "Hello", model, "coder", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody.Model == nil {
		t.Error("expected model to be set")
	} else {
		if receivedBody.Model.ProviderID != "anthropic" {
			t.Errorf("expected providerID 'anthropic', got %s", receivedBody.Model.ProviderID)
		}
	}
	if receivedBody.Agent != "coder" {
		t.Errorf("expected agent 'coder', got %s", receivedBody.Agent)
	}
	if receivedBody.Variant != "default" {
		t.Errorf("expected variant 'default', got %s", receivedBody.Variant)
	}
}

func TestClient_Abort(t *testing.T) {
	aborted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/abort") {
			aborted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
	ctx := context.Background()

	err := client.Abort(ctx, "sess-123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !aborted {
		t.Error("expected abort endpoint to be called")
	}
}

func TestClient_ReplyPermission(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		message *string
	}{
		{
			name:    "allow once",
			reply:   PermissionReplyOnce,
			message: nil,
		},
		{
			name:    "reject with message",
			reply:   PermissionReplyReject,
			message: strPtr("User denied"),
		},
		{
			name:    "reject without message",
			reply:   PermissionReplyReject,
			message: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody PermissionReplyRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewClient(server.URL, "/workspace", "test-password", newTestLogger())
			ctx := context.Background()

			err := client.ReplyPermission(ctx, "perm-123", tt.reply, tt.message)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if receivedBody.Reply != tt.reply {
				t.Errorf("expected reply %s, got %s", tt.reply, receivedBody.Reply)
			}

			if tt.message != nil {
				if receivedBody.Message != *tt.message {
					t.Errorf("expected message %s, got %s", *tt.message, receivedBody.Message)
				}
			} else if tt.reply == PermissionReplyReject {
				// Should have default message
				if receivedBody.Message == "" {
					t.Error("expected default message for reject without message")
				}
			}
		})
	}
}

func TestClient_ControlChannel(t *testing.T) {
	client := NewClient("http://localhost:8080", "/workspace", "test-password", newTestLogger())

	ch := client.ControlChannel()
	if ch == nil {
		t.Error("expected control channel to be non-nil")
	}
}

func TestClient_SetEventHandler(t *testing.T) {
	client := NewClient("http://localhost:8080", "/workspace", "test-password", newTestLogger())

	handler := func(event *SDKEventEnvelope) {
		// Handler logic
	}

	client.SetEventHandler(handler)

	// Access handler through the client - this is internal but we can test it works
	client.mu.RLock()
	h := client.eventHandler
	client.mu.RUnlock()

	if h == nil {
		t.Error("expected event handler to be set")
	}
}

func TestClient_Close(t *testing.T) {
	client := NewClient("http://localhost:8080", "/workspace", "test-password", newTestLogger())

	// First close should succeed
	client.Close()

	// Second close should be a no-op
	client.Close()

	// Client should be marked as closed
	client.mu.RLock()
	closed := client.closed
	client.mu.RUnlock()

	if !closed {
		t.Error("expected client to be closed")
	}
}

func TestClient_EventMatchesSession(t *testing.T) {
	client := NewClient("http://localhost:8080", "/workspace", "test-password", newTestLogger())

	tests := []struct {
		name      string
		eventType string
		props     string
		sessionID string
		want      bool
	}{
		{
			name:      "message.updated matches",
			eventType: SDKEventMessageUpdated,
			props:     `{"info":{"sessionID":"sess-123"}}`,
			sessionID: "sess-123",
			want:      true,
		},
		{
			name:      "message.updated doesn't match",
			eventType: SDKEventMessageUpdated,
			props:     `{"info":{"sessionID":"sess-456"}}`,
			sessionID: "sess-123",
			want:      false,
		},
		{
			name:      "message.part.updated matches",
			eventType: SDKEventMessagePartUpdated,
			props:     `{"part":{"sessionID":"sess-123"}}`,
			sessionID: "sess-123",
			want:      true,
		},
		{
			name:      "other event matches",
			eventType: SDKEventSessionIdle,
			props:     `{"sessionID":"sess-123"}`,
			sessionID: "sess-123",
			want:      true,
		},
		{
			name:      "no sessionID in event - matches",
			eventType: SDKEventSessionIdle,
			props:     `{}`,
			sessionID: "sess-123",
			want:      true,
		},
		{
			name:      "nil properties - matches",
			eventType: SDKEventSessionIdle,
			props:     "",
			sessionID: "sess-123",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var props json.RawMessage
			if tt.props != "" {
				props = json.RawMessage(tt.props)
			}

			event := &SDKEventEnvelope{
				Type:       tt.eventType,
				Properties: props,
			}

			got := client.eventMatchesSession(event, tt.sessionID)
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
