package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func strPtr(s string) *string { return &s }

func createTestAgent(t *testing.T, repo interface {
	CreateAgentInstance(context.Context, *models.AgentInstance) error
}, id, wsID string, maxSessions int,
) {
	t.Helper()
	agent := &models.AgentInstance{
		ID:                    id,
		WorkspaceID:           wsID,
		Name:                  "agent-" + id,
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: maxSessions,
		DesiredSkills:         "[]",
		ExecutorPreference:    "{}",
		Permissions:           "{}",
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent %s: %v", id, err)
	}
}

func TestClaimNextEligibleWakeup_Basic(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)

	req := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         `{"task_id":"t1"}`,
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := repo.ClaimNextEligibleWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != req.ID {
		t.Errorf("claimed ID = %q, want %q", claimed.ID, req.ID)
	}
	if claimed.Status != "claimed" {
		t.Errorf("status = %q, want claimed", claimed.Status)
	}
}

func TestClaimNextEligibleWakeup_SkipsBusyAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)

	// Create a claimed wakeup (agent at capacity).
	claimed := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "claimed",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, claimed); err != nil {
		t.Fatalf("create claimed: %v", err)
	}

	// Create a queued wakeup for same agent.
	queued := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_comment",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, queued); err != nil {
		t.Fatalf("create queued: %v", err)
	}

	// Claim should return nothing since agent is at capacity.
	_, err := repo.ClaimNextEligibleWakeup(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for busy agent, got %v", err)
	}
}

func TestClaimNextEligibleWakeup_PicksNextEligible(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)
	createTestAgent(t, repo, "a2", "ws1", 1)

	// a1 is at capacity (has a claimed wakeup).
	atCap := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "claimed",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, atCap); err != nil {
		t.Fatalf("create: %v", err)
	}

	// a1 has a queued wakeup.
	a1Queued := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_comment",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, a1Queued); err != nil {
		t.Fatalf("create: %v", err)
	}

	// a2 has a queued wakeup.
	a2Queued := &models.WakeupRequest{
		AgentInstanceID: "a2",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, a2Queued); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Should claim a2's wakeup since a1 is busy.
	result, err := repo.ClaimNextEligibleWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if result.AgentInstanceID != "a2" {
		t.Errorf("claimed agent = %q, want a2", result.AgentInstanceID)
	}
}

func TestClaimNextEligibleWakeup_SkipsPausedAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create paused agent.
	agent := &models.AgentInstance{
		ID:                    "a1",
		WorkspaceID:           "ws1",
		Name:                  "paused-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusPaused,
		MaxConcurrentSessions: 1,
		DesiredSkills:         "[]",
		ExecutorPreference:    "{}",
		Permissions:           "{}",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	req := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := repo.ClaimNextEligibleWakeup(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for paused agent, got %v", err)
	}
}

func TestCoalesceWakeup(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)

	req := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_comment",
		Payload:         `{"task_id":"t1"}`,
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	coalesced, err := repo.CoalesceWakeup(ctx, "a1", "task_comment", 10, `{"task_id":"t2"}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if !coalesced {
		t.Error("expected coalesce to succeed")
	}

	reqs, _ := repo.ListWakeupRequests(ctx, "ws1")
	if len(reqs) != 1 {
		t.Fatalf("want 1, got %d", len(reqs))
	}
	if reqs[0].CoalescedCount != 2 {
		t.Errorf("coalesced_count = %d, want 2", reqs[0].CoalescedCount)
	}
}

func TestCheckIdempotencyKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)

	req := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
		IdempotencyKey:  strPtr("unique-key-1"),
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	dup, err := repo.CheckIdempotencyKey(ctx, "unique-key-1", 24)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !dup {
		t.Error("expected duplicate=true for existing key")
	}

	dup, err = repo.CheckIdempotencyKey(ctx, "nonexistent-key", 24)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if dup {
		t.Error("expected duplicate=false for nonexistent key")
	}
}

func TestCleanExpired(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)

	req := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Finish it.
	if err := repo.FinishWakeupRequest(ctx, req.ID, "finished"); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Clean with a future cutoff should remove it.
	n, err := repo.CleanExpired(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if n != 1 {
		t.Errorf("cleaned %d, want 1", n)
	}

	reqs, _ := repo.ListWakeupRequests(ctx, "ws1")
	if len(reqs) != 0 {
		t.Errorf("want 0 after clean, got %d", len(reqs))
	}
}

func TestClaimNextEligibleWakeup_SkipsAgentInCooldown(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create agent with 5 second cooldown and recent finish.
	agent := &models.AgentInstance{
		ID:                    "a-cd",
		WorkspaceID:           "ws1",
		Name:                  "cooldown-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
		CooldownSec:           5,
		DesiredSkills:         "[]",
		ExecutorPreference:    "{}",
		Permissions:           "{}",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Set last_wakeup_finished_at to now (within cooldown).
	now := time.Now().UTC()
	if err := repo.UpdateLastWakeupFinished(ctx, "a-cd", now); err != nil {
		t.Fatalf("update finished: %v", err)
	}

	req := &models.WakeupRequest{
		AgentInstanceID: "a-cd",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := repo.ClaimNextEligibleWakeup(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for cooldown agent, got %v", err)
	}
}

func TestClaimNextEligibleWakeup_AllowsAgentPastCooldown(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                    "a-cd2",
		WorkspaceID:           "ws1",
		Name:                  "past-cooldown-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
		CooldownSec:           5,
		DesiredSkills:         "[]",
		ExecutorPreference:    "{}",
		Permissions:           "{}",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Set last_wakeup_finished_at to 10 seconds ago (past cooldown).
	past := time.Now().UTC().Add(-10 * time.Second)
	if err := repo.UpdateLastWakeupFinished(ctx, "a-cd2", past); err != nil {
		t.Fatalf("update finished: %v", err)
	}

	req := &models.WakeupRequest{
		AgentInstanceID: "a-cd2",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := repo.ClaimNextEligibleWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.AgentInstanceID != "a-cd2" {
		t.Errorf("claimed agent = %q, want a-cd2", claimed.AgentInstanceID)
	}
}

func TestClaimNextEligibleWakeup_SkipsScheduledRetryInFuture(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a-retry", "ws1", 1)

	future := time.Now().UTC().Add(10 * time.Minute)
	req := &models.WakeupRequest{
		AgentInstanceID:  "a-retry",
		Reason:           "task_assigned",
		Payload:          "{}",
		Status:           "queued",
		CoalescedCount:   1,
		RetryCount:       1,
		ScheduledRetryAt: &future,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := repo.ClaimNextEligibleWakeup(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for future retry, got %v", err)
	}
}

func TestScheduleRetry_ResetsToQueued(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a-sr", "ws1", 1)

	req := &models.WakeupRequest{
		AgentInstanceID: "a-sr",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Claim it.
	claimed, err := repo.ClaimNextEligibleWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Schedule retry.
	retryAt := time.Now().UTC().Add(2 * time.Minute)
	if err := repo.ScheduleRetry(ctx, claimed.ID, retryAt, 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	// Should not be claimable yet (retry in future).
	_, err = repo.ClaimNextEligibleWakeup(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows (retry in future), got %v", err)
	}
}

func TestRecoverStale(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	createTestAgent(t, repo, "a1", "ws1", 1)

	req := &models.WakeupRequest{
		AgentInstanceID: "a1",
		Reason:          "task_assigned",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Claim it.
	_, err := repo.ClaimWakeupRequest(ctx, "a1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Recover with a future cutoff should reset it.
	n, err := repo.RecoverStale(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if n != 1 {
		t.Errorf("recovered %d, want 1", n)
	}
}
