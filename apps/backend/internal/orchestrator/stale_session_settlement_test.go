package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestSettleStaleSessionSettlesOnlyEligibleExactTurn(t *testing.T) {
	ctx := context.Background()
	svc, repo, request := seedStaleSettlement(t)

	result, err := svc.SettleStaleSession(ctx, request)
	if err != nil {
		t.Fatalf("SettleStaleSession: %v", err)
	}
	if result.Status != "settled" {
		t.Fatalf("settlement status = %q, want settled", result.Status)
	}
	turn, err := repo.GetTurn(ctx, request.TargetTurnID)
	if err != nil || turn.CompletedAt == nil {
		t.Fatalf("captured turn = (%+v, %v), want completed", turn, err)
	}
	intent, err := repo.GetCompletionIntentForTurn(ctx, request.TargetSessionID, request.TargetTurnID)
	if err != nil || intent.State != models.CompletionIntentStateSettled {
		t.Fatalf("completion intent = (%+v, %v), want settled", intent, err)
	}
}

// TestSettleStaleSessionRecordsAuditEventAtomicallyWithSettlement covers the
// atomicity fix: reconcileCompletionIntentLocked used to run the completion
// intent's terminal transition, then SettleStaleSession wrote the
// session_control_events audit row as a separate statement afterward. A
// crash (or any failure) between the two left a durably settled turn with no
// audit trail of who authorized it. The audit event's Result must also match
// the intent's actual terminal state, not merely a caller-supplied literal.
func TestSettleStaleSessionRecordsAuditEventAtomicallyWithSettlement(t *testing.T) {
	ctx := context.Background()
	svc, repo, request := seedStaleSettlement(t)

	result, err := svc.SettleStaleSession(ctx, request)
	if err != nil {
		t.Fatalf("SettleStaleSession: %v", err)
	}
	if result.Status != "settled" {
		t.Fatalf("settlement status = %q, want settled", result.Status)
	}

	var actorTaskID, evidenceCode, auditResult string
	row := repo.DB().QueryRowContext(ctx, `
		SELECT actor_task_id, evidence_code, result FROM session_control_events
		WHERE target_session_id = ? AND target_turn_id = ?
	`, request.TargetSessionID, request.TargetTurnID)
	if err := row.Scan(&actorTaskID, &evidenceCode, &auditResult); err != nil {
		t.Fatalf("query session_control_events: %v", err)
	}
	if actorTaskID != request.ActorTaskID {
		t.Fatalf("audit actor_task_id = %q, want %q", actorTaskID, request.ActorTaskID)
	}
	if evidenceCode != "eligible_completion_intent" {
		t.Fatalf("audit evidence_code = %q, want eligible_completion_intent", evidenceCode)
	}
	if auditResult != string(models.CompletionIntentStateSettled) {
		t.Fatalf("audit result = %q, want %q", auditResult, models.CompletionIntentStateSettled)
	}
}

func TestSettleStaleSessionPropagatesSettlementCommitFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo, request := seedStaleSettlement(t)
	commitErr := errors.New("injected settlement commit failure")
	svc.repo = &failingSettlementCommitRepository{Repository: repo, err: commitErr}

	_, err := svc.SettleStaleSession(ctx, request)
	if !errors.Is(err, commitErr) {
		t.Fatalf("SettleStaleSession error = %v, want commit failure", err)
	}
	if errors.Is(err, ErrStaleSessionNotStale) {
		t.Fatalf("SettleStaleSession error = %v, must not report active_turn/not_stale after mutation", err)
	}

	var refusalCount int
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM session_control_events
		WHERE target_session_id = ? AND target_turn_id = ? AND result = 'not_stale'
	`, request.TargetSessionID, request.TargetTurnID).Scan(&refusalCount); err != nil {
		t.Fatalf("count refusal audits: %v", err)
	}
	if refusalCount != 0 {
		t.Fatalf("refusal audit count = %d, want 0", refusalCount)
	}
}

type failingSettlementCommitRepository struct {
	*sqliterepo.Repository
	err error
}

func (r *failingSettlementCommitRepository) TransitionCompletionIntentWithControlEvent(
	context.Context,
	string,
	models.CompletionIntentState,
	models.CompletionIntentState,
	time.Time,
	*models.SessionControlEvent,
) (bool, error) {
	return false, r.err
}

func TestSettleStaleSessionRefusesActiveAdministrativeOwnership(t *testing.T) {
	tests := []struct {
		name         string
		prepare      func(t *testing.T, svc *Service, repo *sqliterepo.Repository)
		wantEvidence string
	}{
		{
			name: "prompt dispatch pending",
			prepare: func(t *testing.T, _ *Service, repo *sqliterepo.Repository) {
				t.Helper()
				// GetActiveTurnBySessionID's authority predicate excludes a
				// turn that is a pure dispatch-pending reservation with no
				// other evidence it was actually started, so it would
				// otherwise never be found as the active turn at all (and
				// this case would instead hit "successor_or_missing_turn").
				// Also marking it attempted keeps it discoverable while
				// staleSettlementHasPromptReservation still reports
				// prompt_reservation for pending OR attempted.
				if _, _, err := repo.PatchTurnMetadata(context.Background(), "s1", "turn-1",
					map[string]interface{}{
						models.TurnMetaKeyPromptDispatchPending:   true,
						models.TurnMetaKeyPromptDispatchAttempted: true,
					}); err != nil {
					t.Fatalf("patch turn metadata: %v", err)
				}
			},
			wantEvidence: "prompt_reservation",
		},
		{
			name: "pending tool call",
			prepare: func(t *testing.T, _ *Service, repo *sqliterepo.Repository) {
				t.Helper()
				now := time.Now().UTC()
				if err := repo.CreateMessage(context.Background(), &models.Message{
					ID: "tool-pending", TaskSessionID: "s1", TaskID: "t1", TurnID: "turn-1",
					AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall,
					Metadata:  map[string]interface{}{"tool_call_id": "call-1", "status": "running"},
					CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					t.Fatalf("CreateMessage: %v", err)
				}
			},
			wantEvidence: "pending_tool_call",
		},
		{
			name: "background work",
			prepare: func(t *testing.T, svc *Service, _ *sqliterepo.Repository) {
				t.Helper()
				svc.registerBackgroundTask("s1", "background-1")
			},
			wantEvidence: "background_work",
		},
		{
			name: "persisted background work after restart",
			prepare: func(t *testing.T, _ *Service, repo *sqliterepo.Repository) {
				t.Helper()
				if err := repo.SetSessionMetadataKey(context.Background(), "s1", models.SessionMetaKeyBackgroundWorkAttested, true); err != nil {
					t.Fatalf("persist background attestation: %v", err)
				}
			},
			wantEvidence: "background_work_attested",
		},
		{
			name: "cancellation in flight",
			prepare: func(t *testing.T, svc *Service, _ *sqliterepo.Repository) {
				t.Helper()
				svc.cancellationOperations = map[string]*cancelOperation{"s1": {}}
			},
			wantEvidence: "cancellation_in_flight",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, request := seedStaleSettlement(t)
			tt.prepare(t, svc, repo)

			_, evidence := svc.staleSettlementCandidate(ctx, repo, request)
			if evidence != tt.wantEvidence {
				t.Fatalf("candidate evidence = %q, want %q", evidence, tt.wantEvidence)
			}
			_, err := svc.SettleStaleSession(ctx, request)
			if !errors.Is(err, ErrStaleSessionNotStale) {
				t.Fatalf("SettleStaleSession error = %v, want active_turn/not_stale", err)
			}
			turn, err := repo.GetTurn(ctx, request.TargetTurnID)
			if err != nil || turn.CompletedAt != nil {
				t.Fatalf("captured turn = (%+v, %v), want still active", turn, err)
			}
		})
	}
}

func seedStaleSettlement(t *testing.T) (*Service, *sqliterepo.Repository, StaleSessionSettlementRequest) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Add(-models.CompletionIntentQuietGrace)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "s2", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Create actor session: %v", err)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", "IN_PROGRESS")
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.turnService = &repoTurnService{repo: repo}
	return svc, repo, StaleSessionSettlementRequest{
		ActorTaskID: "t1", ActorSessionID: "s2", TargetTaskID: "t1",
		TargetSessionID: "s1", TargetTurnID: "turn-1", AuthorityBasis: "same_task_peer",
	}
}
