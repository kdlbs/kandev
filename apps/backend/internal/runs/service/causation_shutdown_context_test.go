package service

// Internal (white-box) test file: applyCausationLineage and
// checkSelfTriggerAllowance are unexported, so exercising their
// context-cancellation handling directly needs package-level access,
// unlike every other test in this directory (package service_test).

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newShutdownContextTestService builds a minimal Service + repo pair
// with one seeded agent profile ("agent-1", workspace "ws-1"), for
// testing the causation gates' context-cancellation handling directly.
func newShutdownContextTestService(t *testing.T) *Service {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"test-agent", "test-agent", now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, created_at, updated_at, workspace_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"agent-1", "test-agent", "agent-1", "agent-1", now, now, "ws-1",
	); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	return New(officeRepo.RunsRepository(), nil, log, nil)
}

// TestApplyCausationLineage_ContextCanceledDefersWithoutRecordingFailure
// pins the causation.go half of the MAJOR review finding: a shutdown
// context cancellation reading the causing run must not be counted as a
// genuine unreadable-input gate failure (AC-OFFICE-LAUNCH-SAFETY-001.8's
// principle, applied to the same shared gate-outcome-recording
// infrastructure this refusal gate feeds). The read still fails and the
// enqueue is still refused (there is no row to insert), but the outcome
// must not be recorded into rec, since that record eventually drives the
// durable consecutive-failure escalation a graceful shutdown must not
// trip.
func TestApplyCausationLineage_ContextCanceledDefersWithoutRecordingFailure(t *testing.T) {
	svc := newShutdownContextTestService(t)
	ctx := context.Background()

	causing := &models.Run{
		ID:             "causing-run-1",
		AgentProfileID: "agent-1",
		WorkspaceID:    "ws-1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := svc.repo.CreateRun(ctx, causing); err != nil {
		t.Fatalf("seed causing run: %v", err)
	}

	tx, err := svc.repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := &gateOutcomeRecorder{}
	req := QueueRunRequest{
		AgentProfileID: "agent-1",
		Reason:         "child_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-1",
		CausingRunID:   causing.ID,
	}
	res := &causationResolution{WorkspaceID: "ws-1"}

	err = svc.applyCausationLineage(canceledCtx, tx, rec, "agent-1", req, models.ActorKindAgent, res)
	if err == nil {
		t.Fatal("applyCausationLineage returned nil error for a context-canceled read")
	}
	var refusal *RefusalError
	if !isRefusalError(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if refusal.Gate != RefusalCausingRunUnreadable {
		t.Errorf("gate = %q, want %q", refusal.Gate, RefusalCausingRunUnreadable)
	}
	if len(rec.outcomes) != 0 {
		t.Errorf("rec.outcomes = %+v, want empty (a shutdown cancellation must not record a gate outcome)", rec.outcomes)
	}
}

// TestCheckSelfTriggerAllowance_ContextCanceledDefersWithoutRecordingFailure
// is the self-trigger-count counterpart: a canceled context reading the
// self-trigger count must not be recorded as a gate failure either.
func TestCheckSelfTriggerAllowance_ContextCanceledDefersWithoutRecordingFailure(t *testing.T) {
	svc := newShutdownContextTestService(t)
	ctx := context.Background()

	tx, err := svc.repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := &gateOutcomeRecorder{}
	req := QueueRunRequest{
		AgentProfileID: "agent-1",
		Reason:         "self_trigger_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-1",
	}

	err = svc.checkSelfTriggerAllowance(canceledCtx, tx, rec, "agent-1", req, models.ActorKindAgent, "agent-1", "ws-1", "causation-1", 0)
	if err == nil {
		t.Fatal("checkSelfTriggerAllowance returned nil error for a context-canceled read")
	}
	var refusal *RefusalError
	if !isRefusalError(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if refusal.Gate != RefusalSelfTrigger {
		t.Errorf("gate = %q, want %q", refusal.Gate, RefusalSelfTrigger)
	}
	if len(rec.outcomes) != 0 {
		t.Errorf("rec.outcomes = %+v, want empty (a shutdown cancellation must not record a gate outcome)", rec.outcomes)
	}
}

// isRefusalError is errors.As without importing errors twice in this
// small file's test helpers.
func isRefusalError(err error, target **RefusalError) bool {
	re, ok := err.(*RefusalError)
	if !ok {
		return false
	}
	*target = re
	return true
}
