package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// Both delete paths absorb ErrTaskNotFound so a benign cleanup race cannot
// abort an uninstall or fail a plugin's own delete. That absorption is only
// safe if it is narrow: a repository that is genuinely broken (disk error,
// constraint violation, cleanup barrier held) must still be reported, and a
// conversation it could not delete must not be counted as deleted. These
// tests pin the boundary from the failure side, which the hand-written fakes
// cannot reach — acFakeTaskRepo.DeleteTask returns nil for a task it does not
// have, so under a fake every delete looks like a success.

// acFailingDeleteRepo wraps a task repo and substitutes a chosen error for
// DeleteTask on one specific task id, leaving every other operation real.
type acFailingDeleteRepo struct {
	agentConversationTaskRepo
	failTaskID string
	failWith   error
	calls      int
}

func (r *acFailingDeleteRepo) DeleteTask(ctx context.Context, taskID string) error {
	if taskID == r.failTaskID {
		r.calls++
		return r.failWith
	}
	return r.agentConversationTaskRepo.DeleteTask(ctx, taskID)
}

// A repository error that is not "already gone" must still reach the caller.
// Service.Uninstall is fail-visible on DeleteAllForPlugin precisely so a real
// failure is not silently swallowed into an orphaned conversation.
func TestDeleteAllForPluginReportsNonNotFoundErrors(t *testing.T) {
	realRepo, victim, other := newRepoWithTwoConversations(t)
	diskErr := errors.New("disk I/O error")
	failing := &acFailingDeleteRepo{
		agentConversationTaskRepo: realRepo,
		failTaskID:                victim,
		failWith:                  diskErr,
	}
	svc := NewAgentConversationService(failing, realRepo, nil, newACFakeStateRepo(), nil)
	svc.SetDispatcher(newACFakeDispatcher())

	count, err := svc.DeleteAllForPlugin(context.Background(), "plugin-coordinator")
	if err == nil {
		t.Fatal("a genuine repository failure was reported as cleanup success; uninstall would orphan the conversation")
	}
	if !errors.Is(err, diskErr) {
		t.Errorf("error did not wrap the underlying repository failure: %v", err)
	}
	// Cleanup keeps going after a failure so one bad row cannot block the
	// rest, so the healthy conversation is still removed and still counted.
	if count != 1 {
		t.Errorf("count = %d, want 1 (the conversation that really was deleted)", count)
	}
	if !taskExists(t, realRepo, victim) {
		t.Error("the conversation whose delete failed was counted as gone")
	}
	if taskExists(t, realRepo, other) {
		t.Error("cleanup stopped at the failure instead of continuing past it")
	}
}

// The plugin-facing Delete has the same boundary: already-gone is success,
// genuinely broken is not.
func TestDeleteReportsNonNotFoundErrors(t *testing.T) {
	realRepo, victim, _ := newRepoWithTwoConversations(t)
	constraintErr := errors.New("FOREIGN KEY constraint failed")
	failing := &acFailingDeleteRepo{
		agentConversationTaskRepo: realRepo,
		failTaskID:                victim,
		failWith:                  constraintErr,
	}
	svc := NewAgentConversationService(failing, realRepo, nil, newACFakeStateRepo(), nil)
	svc.SetDispatcher(newACFakeDispatcher())

	count, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-one", "coordinator")
	if err == nil {
		t.Fatal("a genuine repository failure was reported to the plugin as a successful delete")
	}
	if !errors.Is(err, constraintErr) {
		t.Errorf("error did not wrap the underlying repository failure: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (nothing was actually deleted)", count)
	}
	if !taskExists(t, realRepo, victim) {
		t.Error("the conversation whose delete failed was counted as gone")
	}
}

// The guard must key on the sentinel, not on the words in the message. An
// unrelated failure whose text happens to read like "task not found" is still
// a failure, and a string-matching regression here would silently turn real
// errors into success on the uninstall path.
func TestDeletePathsMatchSentinelNotErrorText(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"plain text lookalike", errors.New("task not found: upstream service unavailable")},
		{"wrapped text lookalike", fmt.Errorf("scheduler: %w", errors.New("task not found"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			realRepo, victim, _ := newRepoWithTwoConversations(t)
			failing := &acFailingDeleteRepo{
				agentConversationTaskRepo: realRepo,
				failTaskID:                victim,
				failWith:                  tc.err,
			}
			svc := NewAgentConversationService(failing, realRepo, nil, newACFakeStateRepo(), nil)
			svc.SetDispatcher(newACFakeDispatcher())

			if _, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-one", "coordinator"); err == nil {
				t.Error("Delete absorbed an error that only looks like the not-found sentinel")
			}
			if _, err := svc.DeleteAllForPlugin(context.Background(), "plugin-coordinator"); err == nil {
				t.Error("DeleteAllForPlugin absorbed an error that only looks like the not-found sentinel")
			}
		})
	}
}

// The real sentinel, however wrapped, is absorbed on both paths.
func TestDeletePathsAbsorbWrappedNotFoundSentinel(t *testing.T) {
	realRepo, victim, _ := newRepoWithTwoConversations(t)
	failing := &acFailingDeleteRepo{
		agentConversationTaskRepo: realRepo,
		failTaskID:                victim,
		failWith:                  fmt.Errorf("delete task %s: %w", victim, taskrepo.ErrTaskNotFound),
	}
	svc := NewAgentConversationService(failing, realRepo, nil, newACFakeStateRepo(), nil)
	svc.SetDispatcher(newACFakeDispatcher())

	count, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-one", "coordinator")
	if err != nil {
		t.Fatalf("a wrapped not-found sentinel was reported as a failure: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0: a row this caller did not delete must not be counted", count)
	}
	if failing.calls != 1 {
		t.Errorf("DeleteTask calls on the raced task = %d, want 1", failing.calls)
	}
}

// newRepoWithTwoConversations seeds one workspace holding two managed
// conversations for plugin-coordinator under different keys, and returns the
// repository plus the task ids of the "coordinator" and "second" ones.
func newRepoWithTwoConversations(t *testing.T) (repo *sqliterepo.Repository, victimID, otherID string) {
	t.Helper()
	svc, real := newAgentConversationServiceOverRealRepo(t)
	seedConversationWorkspace(t, real, "ws-one")
	victim := ensureConversation(t, svc, "plugin-coordinator", "ws-one", "coordinator")
	other := ensureConversation(t, svc, "plugin-coordinator", "ws-one", "second")
	return real, victim.TaskID, other.TaskID
}

// compile-time guard: the wrapper must satisfy the interface it embeds.
var _ agentConversationTaskRepo = (*acFailingDeleteRepo)(nil)

type acRecordingTaskDeleter struct {
	delete func(context.Context, string) error
	calls  []string
	err    error
}

func (d *acRecordingTaskDeleter) DeleteTask(ctx context.Context, id string) error {
	d.calls = append(d.calls, id)
	if d.delete != nil {
		return d.delete(ctx, id)
	}
	return d.err
}

func TestDeleteUsesLifecycleAwareTaskDeleterWhenAvailable(t *testing.T) {
	realRepo, victim, _ := newRepoWithTwoConversations(t)
	svc := NewAgentConversationService(realRepo, realRepo, nil, newACFakeStateRepo(), nil)
	svc.SetDispatcher(newACFakeDispatcher())
	deleter := &acRecordingTaskDeleter{delete: realRepo.DeleteTask}
	svc.SetTaskDeleter(deleter)

	count, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-one", "coordinator")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if len(deleter.calls) != 1 || deleter.calls[0] != victim {
		t.Fatalf("deleter calls = %#v, want [%q]", deleter.calls, victim)
	}
	if taskExists(t, realRepo, victim) {
		t.Fatal("lifecycle deleter did not remove the managed conversation")
	}
}
