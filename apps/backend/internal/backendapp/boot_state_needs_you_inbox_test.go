package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/webapp"
)

type needsYouInboxBootPayload struct {
	NeedsYouInboxBoot *struct {
		WorkspaceID      string  `json:"workspaceId"`
		Count            int     `json:"count"`
		HasMore          bool    `json:"hasMore"`
		NextSnoozeExpiry *string `json:"nextSnoozeExpiry"`
	} `json:"needsYouInboxBoot"`
}

func decodeNeedsYouInboxBoot(t *testing.T, state map[string]any) needsYouInboxBootPayload {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal state: %v", err)
	}
	var decoded needsYouInboxBootPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}
	return decoded
}

func TestBootNeedsYouInbox_FlagOff_KeyAbsent(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Only"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	params := routeParams{
		taskSvc:  harness.taskSvc,
		taskRepo: harness.taskRepo,
		userCtrl: harness.userCtrl,
		features: config.FeaturesConfig{NeedsYouInbox: false},
	}
	state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{Route: webapp.RouteHome})

	decoded := decodeNeedsYouInboxBoot(t, state)
	if decoded.NeedsYouInboxBoot != nil {
		t.Fatalf("needsYouInboxBoot = %+v, want absent when the flag is off (AC .13: absent, not zero)", decoded.NeedsYouInboxBoot)
	}
}

func TestBootNeedsYouInbox_FlagOnNoWorkspace_KeyAbsent(t *testing.T) {
	harness := newBootStateTestHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No taskSvc: resolveQuickChatWorkspaceID's fallback chain requires it and
	// bails to "" without one, and no route-specific state ever populates
	// state["workspaces"] either -- the one way this design says the key
	// should come out absent (no active workspace resolves).
	params := routeParams{
		taskRepo: harness.taskRepo,
		features: config.FeaturesConfig{NeedsYouInbox: true},
	}
	state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{Route: webapp.RouteHome})

	decoded := decodeNeedsYouInboxBoot(t, state)
	if decoded.NeedsYouInboxBoot != nil {
		t.Fatalf("needsYouInboxBoot = %+v, want absent when no active workspace resolves", decoded.NeedsYouInboxBoot)
	}
}

func TestBootNeedsYouInbox_FlagOnWithWorkspace_SeedsBoundedZeroCount(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Only"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	params := routeParams{
		taskSvc:  harness.taskSvc,
		taskRepo: harness.taskRepo,
		userCtrl: harness.userCtrl,
		features: config.FeaturesConfig{NeedsYouInbox: true},
	}
	state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{Route: webapp.RouteHome})

	decoded := decodeNeedsYouInboxBoot(t, state)
	if decoded.NeedsYouInboxBoot == nil {
		t.Fatal("needsYouInboxBoot absent, want it present once the flag is on and a workspace resolves")
	}
	boot := decoded.NeedsYouInboxBoot
	if boot.WorkspaceID != "ws-1" {
		t.Errorf("workspaceId = %q, want ws-1 (the sole/resolved workspace)", boot.WorkspaceID)
	}
	if boot.Count != 0 || boot.HasMore {
		t.Errorf("count/hasMore = %d/%v, want 0/false against an empty workspace", boot.Count, boot.HasMore)
	}
	if boot.NextSnoozeExpiry != nil {
		t.Errorf("nextSnoozeExpiry = %v, want nil against an empty workspace", *boot.NextSnoozeExpiry)
	}
}
