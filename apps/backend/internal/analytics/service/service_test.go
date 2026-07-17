package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

// fakeRepository is a minimal repository.Repository stub for exercising
// Service's delegation. Only ListSessionCodeStats is meaningfully
// implemented; the rest satisfy the interface and are unused by these tests.
type fakeRepository struct {
	filter      models.SessionCodeStatsFilter
	stats       []*models.SessionCodeStats
	err         error
	filterCalls int
}

func (f *fakeRepository) GetTaskStats(context.Context, string, *time.Time, int) ([]*models.TaskStats, error) {
	return nil, nil
}

func (f *fakeRepository) GetGlobalStats(context.Context, string, *time.Time) (*models.GlobalStats, error) {
	return nil, nil
}

func (f *fakeRepository) GetDailyActivity(context.Context, string, int) ([]*models.DailyActivity, error) {
	return nil, nil
}

func (f *fakeRepository) GetCompletedTaskActivity(context.Context, string, int) ([]*models.CompletedTaskActivity, error) {
	return nil, nil
}

func (f *fakeRepository) GetAgentUsage(context.Context, string, int, *time.Time) ([]*models.AgentUsage, error) {
	return nil, nil
}

func (f *fakeRepository) GetRepositoryStats(context.Context, string, *time.Time) ([]*models.RepositoryStats, error) {
	return nil, nil
}

func (f *fakeRepository) GetGitStats(context.Context, string, *time.Time) (*models.GitStats, error) {
	return nil, nil
}

func (f *fakeRepository) ListSessionCodeStats(
	_ context.Context, filter models.SessionCodeStatsFilter,
) ([]*models.SessionCodeStats, error) {
	f.filterCalls++
	f.filter = filter
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

func TestService_ListSessionCodeStats_DelegatesToRepositoryAndReturnsResults(t *testing.T) {
	want := []*models.SessionCodeStats{
		{SessionID: "sess-1", LinesAddedCommitted: 10, LinesDeletedCommitted: 2, LinesAddedPeakPending: 5, LinesDeletedPeakPending: 1},
	}
	repo := &fakeRepository{stats: want}
	svc := New(repo)

	filter := models.SessionCodeStatsFilter{WorkspaceIDs: []string{"ws-1"}}
	got, err := svc.ListSessionCodeStats(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if repo.filterCalls != 1 {
		t.Fatalf("expected repository to be called once, got %d", repo.filterCalls)
	}
	if len(repo.filter.WorkspaceIDs) != 1 || repo.filter.WorkspaceIDs[0] != "ws-1" {
		t.Errorf("expected filter to be passed through unchanged, got %+v", repo.filter)
	}
	if len(got) != 1 || got[0].SessionID != "sess-1" {
		t.Errorf("expected repository results to be returned unchanged, got %+v", got)
	}
}

func TestService_ListSessionCodeStats_PropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeRepository{err: wantErr}
	svc := New(repo)

	_, err := svc.ListSessionCodeStats(context.Background(), models.SessionCodeStatsFilter{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v to propagate, got %v", wantErr, err)
	}
}
