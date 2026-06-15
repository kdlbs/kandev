package watchreset

import (
	"context"
	"errors"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

type fakeResetter struct {
	ids      []string
	listErr  error
	cleared  bool
	clearErr error
}

func (f *fakeResetter) ListTaskIDs(_ context.Context) ([]string, error) {
	return f.ids, f.listErr
}
func (f *fakeResetter) Clear(_ context.Context) error {
	f.cleared = true
	return f.clearErr
}

type fakeDeleter struct {
	called  []string
	failOn  map[string]error
	outcome *taskservice.CascadeOutcome
}

func (f *fakeDeleter) DeleteTaskTree(_ context.Context, id string, _ bool) (*taskservice.CascadeOutcome, error) {
	f.called = append(f.called, id)
	if err, ok := f.failOn[id]; ok {
		return nil, err
	}
	return f.outcome, nil
}

func TestRun_DeletesEveryNonEmptyIDAndClears(t *testing.T) {
	r := &fakeResetter{ids: []string{"a", "b", "", "c"}}
	td := &fakeDeleter{}
	res, err := Run(context.Background(), r, td, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TasksDeleted != 3 {
		t.Fatalf("TasksDeleted=%d want 3", res.TasksDeleted)
	}
	if got := len(td.called); got != 3 {
		t.Fatalf("DeleteTaskTree calls=%d want 3", got)
	}
	if !r.cleared {
		t.Fatalf("Clear not called")
	}
}

func TestRun_ContinuesOnDeleteFailure(t *testing.T) {
	r := &fakeResetter{ids: []string{"ok1", "bad", "ok2"}}
	td := &fakeDeleter{failOn: map[string]error{"bad": errors.New("boom")}}
	res, err := Run(context.Background(), r, td, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TasksDeleted != 2 {
		t.Fatalf("TasksDeleted=%d want 2 (skip failing)", res.TasksDeleted)
	}
	if !r.cleared {
		t.Fatalf("Clear must still run even when a delete failed")
	}
}

func TestRun_PropagatesListError(t *testing.T) {
	r := &fakeResetter{listErr: errors.New("db down")}
	td := &fakeDeleter{}
	_, err := Run(context.Background(), r, td, nil)
	if err == nil {
		t.Fatalf("expected list error to surface")
	}
}

func TestRun_PropagatesClearError(t *testing.T) {
	r := &fakeResetter{ids: []string{"a"}, clearErr: errors.New("write fail")}
	td := &fakeDeleter{}
	res, err := Run(context.Background(), r, td, nil)
	if err == nil {
		t.Fatalf("expected clear error to surface")
	}
	if res.TasksDeleted != 1 {
		t.Fatalf("partial TasksDeleted should still be reported, got %d", res.TasksDeleted)
	}
}

func TestPreview_CountsOnlyNonEmptyIDs(t *testing.T) {
	r := &fakeResetter{ids: []string{"", "a", "", "b", "c"}}
	n, err := Preview(context.Background(), r)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if n != 3 {
		t.Fatalf("n=%d want 3", n)
	}
}

func TestRun_RejectsNilArgs(t *testing.T) {
	if _, err := Run(context.Background(), nil, &fakeDeleter{}, nil); err == nil {
		t.Fatalf("nil Resetter must error")
	}
	if _, err := Run(context.Background(), &fakeResetter{}, nil, nil); err == nil {
		t.Fatalf("nil TaskDeleter must error")
	}
}
