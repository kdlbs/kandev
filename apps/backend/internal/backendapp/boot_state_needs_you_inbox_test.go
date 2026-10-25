package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	userstore "github.com/kandev/kandev/internal/user/store"
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

// seedInboxAnswerableBundle creates one answerable clarification bundle (a
// task, session, current turn, and a clarification_request message) in
// workspaceID -- the exact shape ListUnresolvedClarificationBundles requires:
// a resolvable pending_id and question_id, no parent_question marker, the
// message on the session's current (open) turn, and a non-terminal session.
func seedInboxAnswerableBundle(t *testing.T, repo *sqlitetaskrepo.Repository, n int, workspaceID string, at time.Time) (pendingID string) {
	t.Helper()
	ctx := context.Background()
	id := fmt.Sprintf("inbox-%d", n)
	pendingID = "pending-" + id
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-" + id, WorkspaceID: workspaceID, Title: "inbox task"}); err != nil {
		t.Fatalf("CreateTask %s: %v", id, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-" + id, TaskID: "task-" + id}); err != nil {
		t.Fatalf("CreateTaskSession %s: %v", id, err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-" + id, TaskSessionID: "session-" + id, TaskID: "task-" + id,
		StartedAt: at, CreatedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatalf("CreateTurn %s: %v", id, err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "msg-" + id, TaskSessionID: "session-" + id, TaskID: "task-" + id, TurnID: "turn-" + id,
		AuthorType: models.MessageAuthorAgent, Content: "q", Type: models.MessageTypeClarificationRequest,
		RequestsInput: true,
		Metadata: map[string]interface{}{
			"pending_id":  pendingID,
			"question_id": "q1",
			"question": map[string]interface{}{
				"id":     "q1",
				"title":  "title",
				"prompt": "prompt",
			},
		},
		CreatedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatalf("CreateMessage %s: %v", id, err)
	}
	return pendingID
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
	ctx := context.Background()
	// newBootStateTestHarness's fresh repository auto-provisions one unowned
	// workspace (ensureDefaultWorkspace), and an unowned workspace stays
	// visible to every caller until claimed. So the real "no active
	// workspace resolves" path isn't an empty workspace table -- it's a
	// scoped, authenticated caller who owns nothing and has no membership,
	// once that default workspace has been claimed by somebody else (the same
	// ClaimUnownedWorkspaces the auth setup wizard uses to promote the
	// single-user instance's admin).
	if err := harness.taskRepo.ClaimUnownedWorkspaces(ctx, "workspace-owner"); err != nil {
		t.Fatalf("ClaimUnownedWorkspaces: %v", err)
	}

	reqCtx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "outsider", Role: authn.RoleMember})
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(reqCtx)
	params := routeParams{
		taskSvc:  harness.taskSvc,
		taskRepo: harness.taskRepo,
		userCtrl: harness.userCtrl,
		features: config.FeaturesConfig{NeedsYouInbox: true},
	}
	state := bootInitialState(reqCtx, req, params, webapp.RouteClassification{Route: webapp.RouteHome})

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

// TestBootNeedsYouInbox_FlagOnWithBundles_SeedsRealCounts guards against a
// hardcoded-zero implementation: it seeds one more answerable bundle than the
// producer's bounded page limit (default 50, mirrored from defaultInboxLimit)
// plus one snoozed bundle, so count, hasMore, and nextSnoozeExpiry must all
// come from a real read. It also exercises formatOptionalBootTime's non-nil
// branch, which the zero-count test above cannot reach.
func TestBootNeedsYouInbox_FlagOnWithBundles_SeedsRealCounts(t *testing.T) {
	const defaultInboxLimit = 50
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Only"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	now := time.Now().UTC()
	for i := 0; i < defaultInboxLimit+1; i++ {
		seedInboxAnswerableBundle(t, harness.taskRepo, i, "ws-1", now.Add(time.Duration(i)*time.Second))
	}
	snoozedPendingID := seedInboxAnswerableBundle(t, harness.taskRepo, defaultInboxLimit+1, "ws-1", now.Add(time.Hour))
	// The production write path (httpUpsertInboxSidecar) stamps snooze_until
	// from the handler's own unmodified time.Now() clock, and every reader
	// (including this boot-hydration producer) compares against that same
	// unmodified time.Now() -- so the fixture must match that convention
	// rather than normalizing to UTC, or the stored and compared timestamps
	// carry different zone offsets and the TEXT-column ">" comparison stops
	// being chronological. 24 hours out (not 1) keeps the expiry clear of a
	// host's DST fall-back window, where a 1-hour offset can render to nearly
	// the same local wall-clock text as the comparison time.
	snoozeUntil := time.Now().Add(24 * time.Hour)
	if err := harness.taskRepo.UpsertClarificationInboxSidecar(
		ctx, userstore.DefaultUserID, snoozedPendingID, models.ClarificationSidecarSnoozed, &snoozeUntil, now,
	); err != nil {
		t.Fatalf("UpsertClarificationInboxSidecar: %v", err)
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
	// defaultInboxLimit answerable bundles are visible; the snoozed one is
	// hidden from the count but still owns the earliest snooze expiry.
	if boot.Count != defaultInboxLimit {
		t.Errorf("count = %d, want %d (bounded page of the answerable bundles)", boot.Count, defaultInboxLimit)
	}
	if !boot.HasMore {
		t.Error("hasMore = false, want true: more answerable bundles exist beyond the bounded page")
	}
	if boot.NextSnoozeExpiry == nil {
		t.Fatal("nextSnoozeExpiry = nil, want the snoozed bundle's expiry")
	}
	gotExpiry, err := time.Parse(time.RFC3339, *boot.NextSnoozeExpiry)
	if err != nil {
		t.Fatalf("nextSnoozeExpiry %q not RFC3339: %v", *boot.NextSnoozeExpiry, err)
	}
	// RFC3339 (unlike RFC3339Nano) drops sub-second precision, so compare at
	// second granularity rather than requiring an exact instant match.
	if !gotExpiry.Equal(snoozeUntil.Truncate(time.Second)) {
		t.Errorf("nextSnoozeExpiry = %v, want %v", gotExpiry, snoozeUntil)
	}
}
