package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type planCommentServiceContract interface {
	ListPlanComments(context.Context, string) (*models.TaskPlanCommentSnapshot, error)
	CreatePlanComment(context.Context, CreatePlanCommentRequest) (*models.TaskPlanCommentSnapshot, error)
	UpdatePlanComment(context.Context, UpdatePlanCommentRequest) (*models.TaskPlanCommentSnapshot, error)
	DeletePlanComment(context.Context, DeletePlanCommentRequest) (*models.TaskPlanCommentSnapshot, error)
}

func requirePlanCommentService(t *testing.T, service *PlanService) planCommentServiceContract {
	t.Helper()
	contract, ok := any(service).(planCommentServiceContract)
	if !ok {
		t.Fatal("PlanService does not implement task plan comment operations")
	}
	return contract
}

func TestPlanCommentServiceAuthorizesPublishesAndMapsConflicts(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-comment-service")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-comment-service", Content: "A plan", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	plan, err := svc.GetPlan(ctx, "task-comment-service")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	eventBus.ClearEvents()

	var authorized []string
	svc.SetTaskAuthorizer(func(_ context.Context, taskID string) error {
		authorized = append(authorized, taskID)
		if taskID == "task-denied" {
			return errors.New("denied")
		}
		return nil
	})
	comments := requirePlanCommentService(t, svc)
	created, err := comments.CreatePlanComment(ctx, CreatePlanCommentRequest{
		TaskID: "task-comment-service", PlanID: plan.ID,
		ID: "d8d97d5b-2663-45aa-aad4-64580a9ae07f", Body: "Please explain",
		SelectedText: "plan", AnchorFrom: 1, AnchorTo: 5,
	})
	if err != nil {
		t.Fatalf("CreatePlanComment: %v", err)
	}
	if created.Revision != 1 || len(created.Comments) != 1 {
		t.Fatalf("created snapshot = %#v", created)
	}
	if len(authorized) != 1 || authorized[0] != "task-comment-service" {
		t.Fatalf("authorized task IDs = %v", authorized)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != events.TaskPlanCommentsChanged {
		t.Fatalf("published events = %#v, want one comments-changed event", published)
	}
	if payload, ok := published[0].Data.(*models.TaskPlanCommentSnapshot); !ok || payload.Revision != 1 {
		t.Fatalf("event payload = %#v, want revision-1 snapshot", published[0].Data)
	}

	eventBus.ClearEvents()
	_, err = comments.UpdatePlanComment(ctx, UpdatePlanCommentRequest{
		TaskID: "task-comment-service", PlanID: plan.ID,
		ID: created.Comments[0].ID, Body: "stale", ExpectedVersion: 9,
	})
	if !errors.Is(err, ErrTaskPlanCommentsChanged) {
		t.Fatalf("stale update error = %v, want ErrTaskPlanCommentsChanged", err)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatal("failed mutation published an event")
	}

	if _, err := comments.ListPlanComments(ctx, "task-denied"); err == nil || errors.Is(err, repository.ErrTaskPlanNotFound) {
		t.Fatalf("denied list error = %v, want authorizer error", err)
	}
}

func TestPlanCommentServiceValidatesRequests(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	comments := requirePlanCommentService(t, svc)
	ctx := context.Background()

	if _, err := comments.ListPlanComments(ctx, ""); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("empty list task error = %v", err)
	}
	seedTask(t, ctx, repo, "task-without-plan")
	if _, err := comments.ListPlanComments(ctx, "task-without-plan"); !errors.Is(err, ErrTaskPlanNotFound) {
		t.Fatalf("missing plan list error = %v, want ErrTaskPlanNotFound", err)
	}
	if _, err := comments.CreatePlanComment(ctx, CreatePlanCommentRequest{TaskID: "task"}); !errors.Is(err, ErrPlanIDRequired) {
		t.Fatalf("create validation error = %v", err)
	}
	if _, err := comments.UpdatePlanComment(ctx, UpdatePlanCommentRequest{
		TaskID: "task", PlanID: "plan", ID: "d8d97d5b-2663-45aa-aad4-64580a9ae07f", Body: "body",
	}); !errors.Is(err, ErrPlanCommentVersionNeeded) {
		t.Fatalf("update version validation error = %v", err)
	}
	if _, err := comments.DeletePlanComment(ctx, DeletePlanCommentRequest{
		TaskID: "task", PlanID: "plan", ID: "d8d97d5b-2663-45aa-aad4-64580a9ae07f",
	}); !errors.Is(err, ErrPlanCommentVersionNeeded) {
		t.Fatalf("delete version validation error = %v", err)
	}
}
