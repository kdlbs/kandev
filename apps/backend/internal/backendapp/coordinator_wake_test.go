package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeWakeTasks struct {
	task       *taskmodels.Task
	taskErr    error
	primary    *taskmodels.TaskSession
	primaryErr error
	session    *taskmodels.TaskSession
	sessionErr error
	pending    []*taskmodels.Interaction
	pendingErr error
	filter     taskmodels.PendingInteractionFilter
	summaries  map[string]*statussummary.TaskStatusSummary
}

func (f *fakeWakeTasks) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return f.task, f.taskErr
}

func (f *fakeWakeTasks) GetPrimarySession(context.Context, string) (*taskmodels.TaskSession, error) {
	return f.primary, f.primaryErr
}

func (f *fakeWakeTasks) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return f.session, f.sessionErr
}

func (f *fakeWakeTasks) ListPendingInteractions(_ context.Context, filter taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error) {
	f.filter = filter
	return f.pending, f.pendingErr
}

func (f *fakeWakeTasks) GetTaskStatusSummaries(context.Context, []string) (map[string]*statussummary.TaskStatusSummary, error) {
	return f.summaries, nil
}

func TestWakeReader_PrimarySessionID(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	cases := []struct {
		name    string
		tasks   *fakeWakeTasks
		want    string
		wantErr error
	}{
		{"found", &fakeWakeTasks{primary: &taskmodels.TaskSession{ID: "s1"}}, "s1", nil},
		{"none", &fakeWakeTasks{primaryErr: repoerrors.ErrNoPrimarySession}, "", nil},
		{"missing", &fakeWakeTasks{primaryErr: taskmodels.ErrTaskSessionNotFound}, "", nil},
		{"read error", &fakeWakeTasks{primaryErr: boom}, "", boom},
	}
	for _, c := range cases {
		got, err := (&coordinatorWakeReader{tasks: c.tasks}).PrimarySessionID(ctx, "t")
		if got != c.want || !errors.Is(err, c.wantErr) {
			t.Fatalf("%s: got %q, %v", c.name, got, err)
		}
	}
}

func TestWakeReader_PendingInteractions(t *testing.T) {
	ctx := context.Background()
	tasks := &fakeWakeTasks{pending: []*taskmodels.Interaction{{ID: "p1"}, {ID: "p2"}}}
	r := &coordinatorWakeReader{tasks: tasks}
	ids, err := r.PendingPermissionIDs(ctx, "s1")
	if err != nil || len(ids) != 2 || ids[0] != "p1" || ids[1] != "p2" {
		t.Fatalf("permissions = %v, %v", ids, err)
	}
	if len(tasks.filter.Kinds) != 1 || tasks.filter.Kinds[0] != string(taskmodels.InteractionKindPermission) || tasks.filter.SessionIDs[0] != "s1" {
		t.Fatalf("filter = %+v", tasks.filter)
	}
	q, err := r.PendingQuestionID(ctx, "s1")
	if err != nil || q != "p1" || tasks.filter.Kinds[0] != string(taskmodels.InteractionKindClarification) {
		t.Fatalf("question = %q, %v, filter %+v", q, err, tasks.filter)
	}
	tasks.pending = nil
	if q, err := r.PendingQuestionID(ctx, "s1"); q != "" || err != nil {
		t.Fatalf("no question = %q, %v", q, err)
	}
	tasks.pendingErr = errors.New("boom")
	if _, err := r.PendingQuestionID(ctx, "s1"); err == nil {
		t.Fatal("read error must surface")
	}
}

func TestWakeReader_ActiveErrorStamp(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	active := map[string]any{taskmodels.SessionMetaKeyLastAgentError: map[string]any{"message": "boom", "occurred_at": now.Format(time.RFC3339Nano)}}
	dismissed := map[string]any{taskmodels.SessionMetaKeyLastAgentError: map[string]any{"message": "boom", "occurred_at": now.Format(time.RFC3339Nano), "dismissed_at": now.Format(time.RFC3339Nano)}}

	r := &coordinatorWakeReader{tasks: &fakeWakeTasks{session: &taskmodels.TaskSession{Metadata: active}}}
	if stamp, err := r.ActiveErrorStamp(ctx, "s"); err != nil || stamp == "" {
		t.Fatalf("active stamp = %q, %v", stamp, err)
	}
	r = &coordinatorWakeReader{tasks: &fakeWakeTasks{session: &taskmodels.TaskSession{Metadata: dismissed}}}
	if stamp, err := r.ActiveErrorStamp(ctx, "s"); err != nil || stamp != "" {
		t.Fatalf("dismissed stamp = %q, %v", stamp, err)
	}
	r = &coordinatorWakeReader{tasks: &fakeWakeTasks{session: &taskmodels.TaskSession{}}}
	if stamp, err := r.ActiveErrorStamp(ctx, "s"); err != nil || stamp != "" {
		t.Fatalf("no error stamp = %q, %v", stamp, err)
	}
	r = &coordinatorWakeReader{tasks: &fakeWakeTasks{sessionErr: taskmodels.ErrTaskSessionNotFound}}
	if stamp, err := r.ActiveErrorStamp(ctx, "s"); err != nil || stamp != "" {
		t.Fatalf("missing session stamp = %q, %v", stamp, err)
	}
	r = &coordinatorWakeReader{tasks: &fakeWakeTasks{sessionErr: errors.New("boom")}}
	if _, err := r.ActiveErrorStamp(ctx, "s"); err == nil {
		t.Fatal("read error must surface")
	}
}

func TestWakeReader_TaskStateAndLastActivity(t *testing.T) {
	ctx := context.Background()
	at := time.Now().UTC()
	tasks := &fakeWakeTasks{
		task:      &taskmodels.Task{State: v1.TaskStateCompleted},
		summaries: map[string]*statussummary.TaskStatusSummary{"t": {LastActivityAt: &at}},
	}
	r := &coordinatorWakeReader{tasks: tasks}
	if state, err := r.TaskState(ctx, "t"); err != nil || state != "COMPLETED" {
		t.Fatalf("state = %q, %v", state, err)
	}
	if got, err := r.LastActivityAt(ctx, "t"); err != nil || got == nil || !got.Equal(at) {
		t.Fatalf("last activity = %v, %v", got, err)
	}
	if got, err := r.LastActivityAt(ctx, "other"); err != nil || got != nil {
		t.Fatalf("missing summary = %v, %v", got, err)
	}
	tasks.taskErr = repoerrors.ErrTaskNotFound
	if state, err := r.TaskState(ctx, "t"); err != nil || state != "" {
		t.Fatalf("missing task = %q, %v", state, err)
	}
	tasks.taskErr = errors.New("boom")
	if _, err := r.TaskState(ctx, "t"); err == nil {
		t.Fatal("read error must surface")
	}
}
