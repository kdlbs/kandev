package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

func TestGetInbox_IncludesProviderDegradedItems(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	now := time.Now().UTC()
	fake.healthRows = []models.ProviderHealth{
		{
			WorkspaceID: "ws-1",
			ProviderID:  "claude-acp",
			Scope:       "provider",
			ScopeValue:  "",
			State:       "degraded",
			ErrorCode:   "quota_limited",
			RetryAt:     &now,
			LastFailure: &now,
			UpdatedAt:   now,
			RawExcerpt:  "anthropic_quota_exceeded",
		},
	}
	fake.preview = []routing.PreviewItem{
		{
			AgentID: "a1", AgentName: "Alice",
			PrimaryProviderID: "claude-acp",
			Degraded:          true,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/inbox", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.InboxResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, item := range resp.Items {
		if item.Type != "provider_degraded" {
			continue
		}
		found = true
		payload := item.Payload
		if payload["provider_id"] != "claude-acp" {
			t.Errorf("provider_id = %v", payload["provider_id"])
		}
		if payload["action"] != "wait_for_capacity" {
			t.Errorf("action = %v", payload["action"])
		}
		ids, _ := payload["affected_agent_ids"].([]interface{})
		if len(ids) != 1 || ids[0] != "a1" {
			t.Errorf("affected_agent_ids = %v", payload["affected_agent_ids"])
		}
	}
	if !found {
		t.Fatal("expected a provider_degraded inbox item")
	}
}
