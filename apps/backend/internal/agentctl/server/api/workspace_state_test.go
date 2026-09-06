package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestHandleSetPollMode_AcceptsValidModes(t *testing.T) {
	cases := []struct {
		mode string
		want process.PollMode
	}{
		{"fast", process.PollModeFast},
		{"slow", process.PollModeSlow},
		{"paused", process.PollModePaused},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			s := newTestServer(t)

			body, _ := json.Marshal(SetPollModeRequest{Mode: tc.mode})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/poll-mode", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			s.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
			}

			if got := s.procMgr.GetWorkspaceTracker().GetPollMode(); got != tc.want {
				t.Errorf("after request mode=%q, tracker mode = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestHandleSetPollMode_RejectsInvalidMode(t *testing.T) {
	s := newTestServer(t)

	body, _ := json.Marshal(SetPollModeRequest{Mode: "turbo"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/poll-mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid mode, got %d", w.Code)
	}
}

func TestHandleSetPollMode_RejectsInvalidBody(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/poll-mode", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}

func TestHandleSetPollMode_EmptyMode(t *testing.T) {
	s := newTestServer(t)

	body, _ := json.Marshal(SetPollModeRequest{Mode: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/poll-mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty mode, got %d", w.Code)
	}
}

func TestHandleRefreshWorkspace_AcceptsTrigger(t *testing.T) {
	cases := []struct {
		name    string
		trigger string
		want    string
	}{
		{name: "explicit", trigger: "turn_complete", want: "turn_complete"},
		{name: "empty", trigger: "", want: "manual_refresh"},
		{name: "unsupported", trigger: "background_poll", want: "manual_refresh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			body, _ := json.Marshal(RefreshWorkspaceRequest{Trigger: tc.trigger})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/refresh", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			s.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
			}
			var response struct {
				Trigger string `json:"trigger"`
			}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Trigger != tc.want {
				t.Fatalf("response trigger = %q, want %q", response.Trigger, tc.want)
			}
		})
	}
}

func TestHandleRefreshWorkspace_RejectsInvalidBody(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/refresh", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}
