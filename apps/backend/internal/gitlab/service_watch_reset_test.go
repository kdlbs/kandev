package gitlab

import (
	"context"
	"errors"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

type recordingCascadeDeleter struct {
	ids    []string
	failOn string
}

func (d *recordingCascadeDeleter) DeleteTaskTree(_ context.Context, id string, cascade bool) (*taskservice.CascadeOutcome, error) {
	if !cascade {
		panic("watch reset must cascade")
	}
	d.ids = append(d.ids, id)
	if id == d.failOn {
		return nil, errors.New("delete failed")
	}
	return &taskservice.CascadeOutcome{}, nil
}

func TestServiceResetIssueWatchAndDeleteContinueAfterTaskFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &IssueWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		AgentProfileID: "agent-1", ExecutorProfileID: "exec-1", Enabled: true,
	}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	for iid, taskID := range []string{"task-fails", "task-ok"} {
		if ok, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/project", iid+1, "https://gitlab/issue"); err != nil || !ok {
			t.Fatalf("reserve %d: ok=%v err=%v", iid, ok, err)
		}
		if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "group/project", iid+1, taskID); err != nil {
			t.Fatalf("assign %d: %v", iid, err)
		}
	}
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	deleter := &recordingCascadeDeleter{failOn: "task-fails"}
	svc.SetCascadeTaskDeleter(deleter)

	deleted, err := svc.ResetIssueWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if deleted != 1 || len(deleter.ids) != 2 {
		t.Fatalf("deleted=%d calls=%v", deleted, deleter.ids)
	}
	rows, err := store.ListIssueWatchTasksByWatch(ctx, watch.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("dedup after reset=%v err=%v", rows, err)
	}
	gotWatch, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil || gotWatch == nil || !gotWatch.Enabled || gotWatch.LastPolledAt != nil {
		t.Fatalf("watch after reset=%#v err=%v", gotWatch, err)
	}

	if ok, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/project", 3, "https://gitlab/issue"); err != nil || !ok {
		t.Fatalf("reserve after reset: ok=%v err=%v", ok, err)
	}
	if err := svc.DeleteIssueWatch(ctx, watch.ID); err != nil {
		t.Fatalf("delete watch: %v", err)
	}
	gotWatch, err = store.GetIssueWatch(ctx, watch.ID)
	if err != nil || gotWatch != nil {
		t.Fatalf("watch after delete=%#v err=%v", gotWatch, err)
	}
	rows, err = store.ListIssueWatchTasksByWatch(ctx, watch.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("dedup after delete=%v err=%v", rows, err)
	}
}

func TestServiceResetReviewWatchDeletesTasksAndClearsState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &ReviewWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		AgentProfileID: "agent-1", ExecutorProfileID: "exec-1", Enabled: false,
	}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	for iid, taskID := range []string{"task-1", "", "task-2"} {
		if ok, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/project", iid+1, "https://gitlab/mr"); err != nil || !ok {
			t.Fatalf("reserve %d: ok=%v err=%v", iid, ok, err)
		}
		if taskID != "" {
			if err := store.AssignReviewMRTaskID(ctx, watch.ID, "group/project", iid+1, taskID); err != nil {
				t.Fatalf("assign %d: %v", iid, err)
			}
		}
	}

	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	deleter := &recordingCascadeDeleter{}
	svc.SetCascadeTaskDeleter(deleter)

	preview, err := svc.PreviewResetReviewWatch(ctx, watch.ID)
	if err != nil || preview != 2 {
		t.Fatalf("preview=%d err=%v, want 2", preview, err)
	}
	deleted, err := svc.ResetReviewWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if deleted != 2 || len(deleter.ids) != 2 {
		t.Fatalf("deleted=%d calls=%v", deleted, deleter.ids)
	}
	rows, err := store.ListReviewMRTasksByWatch(ctx, watch.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("dedup after reset=%v err=%v", rows, err)
	}
}
