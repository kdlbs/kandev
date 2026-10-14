package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

type cancellingHierarchyReader struct {
	hierarchy.TaskHierarchyReader
	cancel    context.CancelFunc
	readError error
}

type cancellingHierarchyChildrenReader struct {
	hierarchy.TaskHierarchyReader
	cancel    context.CancelFunc
	readError error
}

func (r *cancellingHierarchyChildrenReader) ListChildren(ctx context.Context, id string) ([]*models.Task, error) {
	r.cancel()
	children, err := r.TaskHierarchyReader.ListChildren(ctx, id)
	r.readError = err
	return children, err
}

func TestTaskHierarchyAdmissionChildReadCancellation(t *testing.T) {
	_, bus, repo, create := reparentFixture(t)
	subject, target := create("Subject"), create("Target")
	before, err := repo.GetTask(context.Background(), subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus.ClearEvents()
	var actualReadError error
	err = repo.ValidateTaskParent(ctx, subject.ID, target.ID, func(ctx context.Context, reader hierarchy.TaskHierarchyReader, task *models.Task, parent string) error {
		current := &cancellingHierarchyChildrenReader{TaskHierarchyReader: reader, cancel: cancel}
		result := hierarchy.ValidateParent(ctx, current, task, parent)
		actualReadError = current.readError
		return result
	})
	if !errors.Is(actualReadError, context.Canceled) || !errors.Is(err, actualReadError) || !errors.Is(err, context.Canceled) || errors.Is(err, ErrInvalidParent) {
		t.Fatalf("real child read cancellation=%v; admission error=%v", actualReadError, err)
	}
	current, err := repo.GetTask(context.Background(), subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, current) || len(bus.GetPublishedEvents()) != 0 {
		t.Fatal("cancelled child read changed subject or published success")
	}
}

func TestTaskHierarchyAdmissionChildDecodeAndDepthErrors(t *testing.T) {
	for _, malformed := range []bool{true, false} {
		t.Run(map[bool]string{true: "decode", false: "depth"}[malformed], func(t *testing.T) {
			svc, bus, repo, create := reparentFixture(t)
			subject, target, child := create("Subject"), create("Target"), create("Child")
			ctx := context.Background()
			if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: &subject.ID}); err != nil {
				t.Fatal(err)
			}
			if malformed {
				if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET workflow_agent_overrides='{' WHERE id=?`, child.ID); err != nil {
					t.Fatal(err)
				}
			}
			before, err := repo.GetTask(ctx, subject.ID)
			if err != nil {
				t.Fatal(err)
			}
			bus.ClearEvents()
			_, err = svc.UpdateTask(ctx, subject.ID, &UpdateTaskRequest{ParentID: &target.ID})
			if malformed {
				if !errors.Is(err, models.ErrMalformedWorkflowAgentOverrides) || errors.Is(err, ErrInvalidParent) {
					t.Fatalf("real child decode error identity lost: %v", err)
				}
			} else if !errors.Is(err, ErrInvalidParent) || !errors.Is(err, ErrSubtaskDepthExceeded) {
				t.Fatalf("normal child depth error identities lost: %v", err)
			}
			current, err := repo.GetTask(ctx, subject.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, current) || len(bus.GetPublishedEvents()) != 0 {
				t.Fatal("rejected child read/depth decision changed subject or published success")
			}
		})
	}
}

func (r *cancellingHierarchyReader) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.cancel()
	task, err := r.TaskHierarchyReader.GetTask(ctx, id)
	r.readError = err
	return task, err
}

func TestTaskHierarchyAdmissionDirectParentCancellation(t *testing.T) {
	_, _, repo, create := reparentFixture(t)
	subject, target := create("Subject"), create("Target")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var actualReadError error
	err := repo.ValidateTaskParent(ctx, subject.ID, target.ID, func(ctx context.Context, reader hierarchy.TaskHierarchyReader, task *models.Task, parent string) error {
		current := &cancellingHierarchyReader{TaskHierarchyReader: reader, cancel: cancel}
		result := hierarchy.ValidateParent(ctx, current, task, parent)
		actualReadError = current.readError
		return result
	})
	if !errors.Is(actualReadError, context.Canceled) || !errors.Is(err, actualReadError) || !errors.Is(err, context.Canceled) || errors.Is(err, ErrInvalidParent) {
		t.Fatalf("real direct read cancellation=%v; admission error=%v", actualReadError, err)
	}
	current, err := repo.GetTask(context.Background(), subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ParentID != "" {
		t.Fatal("cancelled direct target read changed subject parent")
	}
}
