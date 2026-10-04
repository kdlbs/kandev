package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

type cancellingHierarchyReader struct {
	hierarchy.TaskHierarchyReader
	cancel    context.CancelFunc
	readError error
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
