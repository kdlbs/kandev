package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func createTestWalkthroughService(t *testing.T) (*WalkthroughService, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewWalkthroughService(repo, eventBus, log)
	return svc, eventBus, repo
}

func sampleSteps() []models.WalkthroughStep {
	return []models.WalkthroughStep{
		{Title: "Entry", File: "main.go", Line: 10, Text: "Program starts here"},
		{File: "server.go", Line: 42, LineEnd: 50, Text: "Routes registered"},
	}
}

func TestWalkthroughService_ShowWalkthrough_Create(t *testing.T) {
	svc, eventBus, repo := createTestWalkthroughService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	wt, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{
		TaskID: "task-1",
		Title:  "Tour",
		Steps:  sampleSteps(),
	})
	if err != nil {
		t.Fatalf("ShowWalkthrough failed: %v", err)
	}
	if wt.TaskID != "task-1" || wt.Title != "Tour" {
		t.Fatalf("unexpected walkthrough: %+v", wt)
	}
	if len(wt.Steps) != 2 || wt.Steps[0].File != "main.go" {
		t.Fatalf("steps not persisted: %+v", wt.Steps)
	}
	if wt.CreatedBy != "agent" {
		t.Fatalf("expected agent author, got %q", wt.CreatedBy)
	}

	if findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskWalkthroughCreated) == nil {
		t.Fatalf("expected %s event", events.TaskWalkthroughCreated)
	}
}

func TestWalkthroughService_ShowWalkthrough_Replace(t *testing.T) {
	svc, eventBus, repo := createTestWalkthroughService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	first, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{TaskID: "task-1", Steps: sampleSteps()})
	if err != nil {
		t.Fatalf("first ShowWalkthrough failed: %v", err)
	}
	eventBus.ClearEvents()

	second, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{
		TaskID: "task-1",
		Steps:  []models.WalkthroughStep{{File: "only.go", Line: 1, Text: "single"}},
	})
	if err != nil {
		t.Fatalf("second ShowWalkthrough failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replace should keep same id: %s vs %s", first.ID, second.ID)
	}
	if len(second.Steps) != 1 || second.Steps[0].File != "only.go" {
		t.Fatalf("steps not replaced: %+v", second.Steps)
	}

	got, err := svc.GetWalkthrough(ctx, "task-1")
	if err != nil || got == nil || len(got.Steps) != 1 {
		t.Fatalf("GetWalkthrough after replace = %+v, err %v", got, err)
	}
	if findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskWalkthroughUpdated) == nil {
		t.Fatalf("expected %s event on replace", events.TaskWalkthroughUpdated)
	}
}

func TestWalkthroughService_ShowWalkthrough_Validation(t *testing.T) {
	svc, _, repo := createTestWalkthroughService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	if _, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{Steps: sampleSteps()}); err == nil {
		t.Fatal("expected error for missing task_id")
	}
	if _, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{TaskID: "task-1"}); err == nil {
		t.Fatal("expected error for empty steps")
	}

	tests := []struct {
		name  string
		steps []models.WalkthroughStep
	}{
		{
			name:  "empty file",
			steps: []models.WalkthroughStep{{File: "  ", Line: 1, Text: "x"}},
		},
		{
			name:  "empty text",
			steps: []models.WalkthroughStep{{File: "a.go", Line: 1, Text: "  "}},
		},
		{
			name:  "non-positive line",
			steps: []models.WalkthroughStep{{File: "a.go", Line: 0, Text: "x"}},
		},
		{
			name:  "line_end before line",
			steps: []models.WalkthroughStep{{File: "a.go", Line: 5, LineEnd: 4, Text: "x"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{TaskID: "task-1", Steps: tt.steps}); err == nil {
				t.Fatalf("expected validation error for %s", tt.name)
			}
		})
	}
}

func TestWalkthroughService_ShowWalkthrough_TrimsInput(t *testing.T) {
	svc, _, repo := createTestWalkthroughService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	wt, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{
		TaskID: "task-1",
		Title:  "  Tour  ",
		Steps: []models.WalkthroughStep{{
			Title: "  Intro  ",
			Repo:  "  repo-a  ",
			File:  "  main.go  ",
			Line:  7,
			Text:  "  explanation  ",
		}},
	})
	if err != nil {
		t.Fatalf("ShowWalkthrough failed: %v", err)
	}
	if wt.Title != "Tour" {
		t.Fatalf("expected trimmed title, got %q", wt.Title)
	}
	step := wt.Steps[0]
	if step.Title != "Intro" || step.Repo != "repo-a" || step.File != "main.go" || step.Text != "explanation" {
		t.Fatalf("expected trimmed step fields, got %+v", step)
	}
}

func TestWalkthroughService_GetMissing(t *testing.T) {
	svc, _, repo := createTestWalkthroughService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	got, err := svc.GetWalkthrough(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetWalkthrough failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing walkthrough, got %+v", got)
	}
}

func TestWalkthroughService_Delete(t *testing.T) {
	svc, eventBus, repo := createTestWalkthroughService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	if _, err := svc.ShowWalkthrough(ctx, ShowWalkthroughRequest{TaskID: "task-1", Steps: sampleSteps()}); err != nil {
		t.Fatalf("seed walkthrough failed: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.DeleteWalkthrough(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteWalkthrough failed: %v", err)
	}
	got, _ := svc.GetWalkthrough(ctx, "task-1")
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
	if findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskWalkthroughDeleted) == nil {
		t.Fatalf("expected %s event", events.TaskWalkthroughDeleted)
	}

	if err := svc.DeleteWalkthrough(ctx, "task-1"); err != ErrTaskWalkthroughNotFound {
		t.Fatalf("expected ErrTaskWalkthroughNotFound, got %v", err)
	}
}
