package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestWsPushForwardsPushOptions proves the ws payload's two optional inputs
// reach the agentctl request body together, rather than one being dropped.
func TestWsPushForwardsPushOptions(t *testing.T) {
	var body map[string]any
	h, server := gitHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"success":true,"operation":"push","output":"done","pushed_remote":"backup","pushed_branch":"feature/work"}`))
	})
	defer server.Close()

	msg, err := ws.NewRequest("id", "action", GitPushRequest{
		SessionID:      "s",
		Repo:           "repo",
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := h.wsPush(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsPush: %v", err)
	}
	if body["remote"] != "backup" {
		t.Errorf("body[remote] = %#v, want backup", body["remote"])
	}
	if body["expected_branch"] != "feature/work" {
		t.Errorf("body[expected_branch] = %#v, want feature/work", body["expected_branch"])
	}
	// The destination fields must survive the round trip back to the caller.
	payload := string(response.Payload)
	if !strings.Contains(payload, `"pushed_remote":"backup"`) {
		t.Errorf("response payload lost pushed_remote: %s", payload)
	}
	if !strings.Contains(payload, `"pushed_branch":"feature/work"`) {
		t.Errorf("response payload lost pushed_branch: %s", payload)
	}
}
