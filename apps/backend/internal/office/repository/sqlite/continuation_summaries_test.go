package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestContinuationSummary_UpsertAndRead(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	row := sqlite.AgentContinuationSummary{
		AgentProfileID: "agent-1",
		Scope:          "heartbeat",
		Content:        "## Active focus\nWatching CI failures.\n",
		ContentTokens:  20,
		UpdatedAt:      time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		UpdatedByRunID: "run-1",
	}
	if err := repo.UpsertContinuationSummary(ctx, row); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.GetContinuationSummary(ctx, "agent-1", "heartbeat")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != row.Content {
		t.Errorf("content mismatch: got %q, want %q", got.Content, row.Content)
	}
	if got.ContentTokens != 20 {
		t.Errorf("tokens = %d, want 20", got.ContentTokens)
	}
	if got.UpdatedByRunID != "run-1" {
		t.Errorf("updated_by_run_id = %q, want run-1", got.UpdatedByRunID)
	}

	// Update path: same key, different content.
	row.Content = "## Active focus\nNew direction.\n"
	row.ContentTokens = 30
	row.UpdatedByRunID = "run-2"
	if err := repo.UpsertContinuationSummary(ctx, row); err != nil {
		t.Fatalf("update upsert: %v", err)
	}
	got, err = repo.GetContinuationSummary(ctx, "agent-1", "heartbeat")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if !strings.Contains(got.Content, "New direction.") {
		t.Errorf("expected updated content, got %q", got.Content)
	}
	if got.UpdatedByRunID != "run-2" {
		t.Errorf("updated_by_run_id = %q, want run-2", got.UpdatedByRunID)
	}
}

func TestContinuationSummary_GetMissingReturnsErrNoRows(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetContinuationSummary(ctx, "ghost", "heartbeat")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestContinuationSummary_TruncatesAt8KB(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	huge := strings.Repeat("x", 9000)
	row := sqlite.AgentContinuationSummary{
		AgentProfileID: "agent-2",
		Scope:          "heartbeat",
		Content:        huge,
	}
	if err := repo.UpsertContinuationSummary(ctx, row); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetContinuationSummary(ctx, "agent-2", "heartbeat")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Content) != 8192 {
		t.Errorf("content len = %d, want 8192", len(got.Content))
	}
}

func TestContinuationSummary_DistinctScopesCoexist(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	hb := sqlite.AgentContinuationSummary{
		AgentProfileID: "agent-3",
		Scope:          "heartbeat",
		Content:        "heartbeat body",
	}
	rt := sqlite.AgentContinuationSummary{
		AgentProfileID: "agent-3",
		Scope:          "routine:daily",
		Content:        "routine body",
	}
	if err := repo.UpsertContinuationSummary(ctx, hb); err != nil {
		t.Fatalf("upsert hb: %v", err)
	}
	if err := repo.UpsertContinuationSummary(ctx, rt); err != nil {
		t.Fatalf("upsert rt: %v", err)
	}
	gotHB, err := repo.GetContinuationSummary(ctx, "agent-3", "heartbeat")
	if err != nil {
		t.Fatalf("get hb: %v", err)
	}
	if gotHB.Content != "heartbeat body" {
		t.Errorf("hb content mismatch: %q", gotHB.Content)
	}
	gotRT, err := repo.GetContinuationSummary(ctx, "agent-3", "routine:daily")
	if err != nil {
		t.Fatalf("get rt: %v", err)
	}
	if gotRT.Content != "routine body" {
		t.Errorf("rt content mismatch: %q", gotRT.Content)
	}
}
