package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newLifecycleShadowTestService seeds one workspace/workflow/task/session so
// AC-4's scenario has a real session to hang turns and messages off.
func newLifecycleShadowTestService(t *testing.T) (*Service, *sqliterepo.Repository, string, string) {
	t.Helper()
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const wsID, wfID, taskID, sessionID = "ws-cta", "wf-cta", "task-cta", "sess-cta"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "CTA"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: wsID, WorkflowID: wfID, WorkflowStepID: "step-1",
		Title: "CTA", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return svc, repo, taskID, sessionID
}

// TestCurrentTurnAuthoritySurvivesLifecycleShadowOverPendingClarification is
// AC-4: a near-simultaneous lifecycle turn created through the real
// Service.CreateMessage(CompletedTurn: true) write path must never shadow a
// genuinely open turn holding a pending clarification, across every named
// consumer surface (list_pending_questions_kandev, GetPendingActionsBySessionIDs,
// and internal/clarification's respond path). Verified at the service and
// repository layers per the spec's own scoping, not browser-level: the
// respond surface is exercised through CompleteActiveClarificationBundle,
// the exact repository method internal/clarification's resolver calls.
func TestCurrentTurnAuthoritySurvivesLifecycleShadowOverPendingClarification(t *testing.T) {
	svc, repo, taskID, sessionID := newLifecycleShadowTestService(t)
	ctx := context.Background()

	// The conversational turn is seeded already-completed (its agent turn
	// finished; the clarification it raised is still unanswered) and MUST
	// start in the past: createCompletedTurn stamps time.Now().UTC(), so an
	// open conversational turn would already win D1's open-beats-completed
	// tie-break for the wrong reason (R2), without ever exercising R1's
	// lifecycle_only exclusion. Only pitting two completed turns against each
	// other - where D1's ordering alone would let the later (synthetic) one
	// win - proves the exclusion is what saves the message.
	past := time.Now().UTC().Add(-time.Hour)
	const conversationTurnID = "turn-conversation"
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: conversationTurnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: past, CreatedAt: past, CompletedAt: &past, UpdatedAt: past,
	}); err != nil {
		t.Fatalf("seed conversational turn: %v", err)
	}

	const pendingID = "pending-cta"
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "msg-cta-question", TaskSessionID: sessionID, TaskID: taskID,
		TurnID: conversationTurnID, AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{
			"pending_id":     pendingID,
			"question_id":    "q1",
			"status":         "pending",
			"question_index": 0,
			"question_total": 1,
			"question": map[string]interface{}{
				"id":     "q1",
				"title":  "title",
				"prompt": "prompt",
				"options": []map[string]interface{}{
					{"option_id": "opt1", "label": "Yes"},
				},
			},
		},
		CreatedAt: past,
	}); err != nil {
		t.Fatalf("seed pending clarification message: %v", err)
	}

	// R1: reproduce the defect's trigger through the real write path - an
	// agent resume creates a synthetic completed turn, never an inserted row
	// imitating one.
	if _, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		Content:       "resumed",
		CompletedTurn: true,
	}); err != nil {
		t.Fatalf("create lifecycle turn via CreateMessage: %v", err)
	}

	t.Run("list_pending_questions_kandev surface", func(t *testing.T) {
		page, err := repo.ListUnresolvedClarificationBundles(ctx, models.ListClarificationBundlesOptions{
			Unscoped: true, Limit: 10,
		})
		if err != nil {
			t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
		}
		if len(page.Bundles) != 1 || page.Bundles[0].PendingID != pendingID {
			t.Fatalf("bundles = %+v, want exactly pending bundle %s", page.Bundles, pendingID)
		}
	})

	t.Run("GetPendingActionsBySessionIDs surface", func(t *testing.T) {
		actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
		}
		if actions[sessionID] != models.TaskPendingActionClarification {
			t.Fatalf("pending action for %s = %v, want %v", sessionID, actions[sessionID], models.TaskPendingActionClarification)
		}
	})

	t.Run("internal/clarification respond surface", func(t *testing.T) {
		completed, claimed, err := repo.CompleteActiveClarificationBundle(ctx, pendingID, "answered", map[string]interface{}{
			"q1": "opt1",
		})
		if err != nil {
			t.Fatalf("CompleteActiveClarificationBundle: %v", err)
		}
		if !claimed || len(completed) != 1 {
			t.Fatalf("claimed=%v completed=%+v, want a single successful claim, not a conflict", claimed, completed)
		}
	})
}

// TestCurrentTurnAuthoritySurvivesLifecycleShadowOverPendingClarificationOpenTurn
// covers AC-4's literal precondition: the real conversational turn is
// genuinely OPEN (never completed) when the synthetic lifecycle turn is
// created after it. TestCurrentTurnAuthoritySurvivesLifecycleShadowOverPendingClarification
// deliberately completes the conversational turn to isolate R1's
// lifecycle_only exclusion from D1's open-beats-completed tie-break; this
// test instead exercises the scenario as it actually happens in production -
// an agent resume racing a still-open, awaiting-input turn - across the same
// three named consumer surfaces.
func TestCurrentTurnAuthoritySurvivesLifecycleShadowOverPendingClarificationOpenTurn(t *testing.T) {
	svc, repo, taskID, sessionID := newLifecycleShadowTestService(t)
	ctx := context.Background()

	past := time.Now().UTC().Add(-time.Hour)
	const conversationTurnID = "turn-conversation-open"
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: conversationTurnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: past, CreatedAt: past, UpdatedAt: past,
	}); err != nil {
		t.Fatalf("seed open conversational turn: %v", err)
	}

	const pendingID = "pending-cta-open"
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "msg-cta-open-question", TaskSessionID: sessionID, TaskID: taskID,
		TurnID: conversationTurnID, AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{
			"pending_id":     pendingID,
			"question_id":    "q1",
			"status":         "pending",
			"question_index": 0,
			"question_total": 1,
			"question": map[string]interface{}{
				"id":     "q1",
				"title":  "title",
				"prompt": "prompt",
				"options": []map[string]interface{}{
					{"option_id": "opt1", "label": "Yes"},
				},
			},
		},
		CreatedAt: past,
	}); err != nil {
		t.Fatalf("seed pending clarification message: %v", err)
	}

	// R1/D5: reproduce the defect's trigger through the real write path - an
	// agent resume creates a synthetic completed turn after the real turn,
	// while the real turn is still open and awaiting the clarification's
	// answer.
	if _, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		Content:       "resumed",
		CompletedTurn: true,
	}); err != nil {
		t.Fatalf("create lifecycle turn via CreateMessage: %v", err)
	}

	t.Run("list_pending_questions_kandev surface", func(t *testing.T) {
		page, err := repo.ListUnresolvedClarificationBundles(ctx, models.ListClarificationBundlesOptions{
			Unscoped: true, Limit: 10,
		})
		if err != nil {
			t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
		}
		if len(page.Bundles) != 1 || page.Bundles[0].PendingID != pendingID {
			t.Fatalf("bundles = %+v, want exactly pending bundle %s", page.Bundles, pendingID)
		}
	})

	t.Run("GetPendingActionsBySessionIDs surface", func(t *testing.T) {
		actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
		}
		if actions[sessionID] != models.TaskPendingActionClarification {
			t.Fatalf("pending action for %s = %v, want %v", sessionID, actions[sessionID], models.TaskPendingActionClarification)
		}
	})

	t.Run("internal/clarification respond surface", func(t *testing.T) {
		completed, claimed, err := repo.CompleteActiveClarificationBundle(ctx, pendingID, "answered", map[string]interface{}{
			"q1": "opt1",
		})
		if err != nil {
			t.Fatalf("CompleteActiveClarificationBundle: %v", err)
		}
		if !claimed || len(completed) != 1 {
			t.Fatalf("claimed=%v completed=%+v, want a single successful claim, not a conflict", claimed, completed)
		}
	})
}

// TestBootResumeLifecycleTurnNeverClaimsClarificationRequest is AC-7 / D5: the
// boot-on-resume path (Service.CreateMessage with CompletedTurn: true,
// Type: "script_execution", mirroring bootMsgAdapter.CreateMessage in
// internal/backendapp/worktree.go) creates a lifecycle turn, but a
// clarification_request written afterward through the real getOrStartTurn
// path must still land on the open conversational turn, never the lifecycle
// one - regardless of the lifecycle turn's later timestamp.
func TestBootResumeLifecycleTurnNeverClaimsClarificationRequest(t *testing.T) {
	svc, repo, taskID, sessionID := newLifecycleShadowTestService(t)
	ctx := context.Background()

	past := time.Now().UTC().Add(-time.Hour)
	const conversationTurnID = "turn-conversation-ac7"
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: conversationTurnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: past, CreatedAt: past, UpdatedAt: past,
	}); err != nil {
		t.Fatalf("seed open conversational turn: %v", err)
	}

	bootMsg, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		Content:       "",
		AuthorType:    "agent",
		Type:          "script_execution",
		CompletedTurn: true,
		Metadata: map[string]interface{}{
			"script_type": "agent_boot",
			"status":      "running",
			"is_resuming": true,
		},
	})
	if err != nil {
		t.Fatalf("create boot message via CompletedTurn path: %v", err)
	}
	if bootMsg.TurnID == conversationTurnID {
		t.Fatalf("boot message landed on the conversational turn %s, want a fresh lifecycle turn", conversationTurnID)
	}
	lifecycleTurnID := bootMsg.TurnID

	clarification, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		AuthorType:    "agent",
		Type:          string(models.MessageTypeClarificationRequest),
		Content:       "q",
		Metadata: map[string]interface{}{
			"pending_id":  "pending-ac7",
			"question_id": "q1",
			"status":      "pending",
		},
	})
	if err != nil {
		t.Fatalf("create clarification_request after boot resume: %v", err)
	}
	if clarification.TurnID != conversationTurnID {
		t.Fatalf("clarification_request turn_id = %s, want the open conversational turn %s", clarification.TurnID, conversationTurnID)
	}
	if clarification.TurnID == lifecycleTurnID {
		t.Fatalf("clarification_request landed on the lifecycle turn %s, want the open conversational turn", lifecycleTurnID)
	}
}

// TestCreateMessageRequestCompletedTurnNotJSONSettable is AC-7's second half:
// CreateMessageRequest.CompletedTurn is tagged json:"-" so no API client can
// set it. Unmarshalling a body that sets completed_turn SHALL leave the field
// false; a caller later making it settable while writing a
// clarification_request would let any client hide its own question.
func TestCreateMessageRequestCompletedTurnNotJSONSettable(t *testing.T) {
	var req CreateMessageRequest
	body := []byte(`{"session_id":"sess","content":"hi","completed_turn":true}`)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.CompletedTurn {
		t.Fatal("CompletedTurn = true after unmarshalling a body setting completed_turn, want false (json:\"-\")")
	}
}
