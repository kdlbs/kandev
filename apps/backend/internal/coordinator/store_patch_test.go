package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPatchCoordinator_UpdatesSentFieldsOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	newName := "Renamed"
	updated, cleared, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &newName}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinator: %v", err)
	}
	if cleared != nil {
		t.Fatalf("cleared = %v, want nil (no conversation task id was set)", cleared)
	}
	if updated.Name != "Renamed" || updated.AgentProfileID != "a" || updated.ExecutorProfileID != "e" || updated.Context != "orig" {
		t.Fatalf("PatchCoordinator merged unsent fields unexpectedly: %+v", updated)
	}

	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.Name != "Renamed" {
		t.Fatalf("GetCoordinator.Name = %q, want %q", got.Name, "Renamed")
	}
}

func TestPatchCoordinator_UnknownIDIsNotFound(t *testing.T) {
	store := newTestStore(t)
	name := "x"
	if _, _, err := store.PatchCoordinator(context.Background(), "ws-1", "missing", CoordinatorPatch{Name: &name}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PatchCoordinator(missing): err = %v, want ErrNotFound", err)
	}
}

func TestPatchCoordinator_WrongWorkspaceIsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	name := "renamed"
	if _, _, err := store.PatchCoordinator(ctx, "ws-2", c.ID, CoordinatorPatch{Name: &name}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PatchCoordinator across workspaces: err = %v, want ErrNotFound", err)
	}
}

func TestPatchCoordinator_ValidatorErrorAbortsWithoutWriting(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	sentinel := errors.New("profile validation failed")
	name := "renamed"
	_, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, func(_ context.Context, _ *Coordinator) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("PatchCoordinator validate error = %v, want %v", err, sentinel)
	}

	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.Name != "Ops" {
		t.Fatalf("GetCoordinator.Name = %q, want unchanged %q", got.Name, "Ops")
	}
}

func TestPatchCoordinator_ClearsConversationTaskIDOnContextOrProfileChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	taskID := "task-1"
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(
		`UPDATE coordinators SET conversation_task_id = ? WHERE id = ?`), taskID, c.ID); err != nil {
		t.Fatalf("seed conversation_task_id: %v", err)
	}

	newContext := "changed"
	updated, cleared, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Context: &newContext}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinator: %v", err)
	}
	if cleared == nil || *cleared != taskID {
		t.Fatalf("cleared conversation task id = %v, want %q", cleared, taskID)
	}
	if updated.ConversationTaskID != nil {
		t.Fatalf("updated.ConversationTaskID = %v, want nil", updated.ConversationTaskID)
	}

	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.ConversationTaskID != nil {
		t.Fatalf("GetCoordinator.ConversationTaskID = %v, want nil", got.ConversationTaskID)
	}
}

func TestPatchCoordinator_PreservesConversationTaskIDWhenFieldsUnchanged(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	taskID := "task-1"
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(
		`UPDATE coordinators SET conversation_task_id = ? WHERE id = ?`), taskID, c.ID); err != nil {
		t.Fatalf("seed conversation_task_id: %v", err)
	}

	// Same trimmed context and a name-only change: neither context nor
	// either profile id differs from the committed row, so the conversation
	// link is preserved.
	sameContext := "orig"
	newName := "Renamed"
	updated, cleared, err := store.PatchCoordinator(ctx, "ws-1", c.ID,
		CoordinatorPatch{Name: &newName, Context: &sameContext}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinator: %v", err)
	}
	if cleared != nil {
		t.Fatalf("cleared conversation task id = %v, want nil", cleared)
	}
	if updated.ConversationTaskID == nil || *updated.ConversationTaskID != taskID {
		t.Fatalf("updated.ConversationTaskID = %v, want %q", updated.ConversationTaskID, taskID)
	}

	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.ConversationTaskID == nil || *got.ConversationTaskID != taskID {
		t.Fatalf("GetCoordinator.ConversationTaskID = %v, want %q", got.ConversationTaskID, taskID)
	}
}

// TestPatchCoordinator_IncrementsConfigRevisionOnContextOrProfileChange
// covers the maintainer-review config_revision fix
// (docs/specs/coordinator/system-design/coordinators.md#store): a PATCH that
// changes context, agent_profile_id or executor_profile_id increments
// config_revision by exactly one, in the same UPDATE that clears
// conversation_task_id.
func TestPatchCoordinator_IncrementsConfigRevisionOnContextOrProfileChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	if c.ConfigRevision != 0 {
		t.Fatalf("created ConfigRevision = %d, want 0", c.ConfigRevision)
	}

	newContext := "changed"
	updated, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Context: &newContext}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinator: %v", err)
	}
	if updated.ConfigRevision != 1 {
		t.Fatalf("updated.ConfigRevision = %d, want 1", updated.ConfigRevision)
	}

	got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
	if err != nil {
		t.Fatalf("GetCoordinator: %v", err)
	}
	if got.ConfigRevision != 1 {
		t.Fatalf("GetCoordinator.ConfigRevision = %d, want 1", got.ConfigRevision)
	}
}

// TestPatchCoordinator_PreservesConfigRevisionWhenFieldsUnchanged covers the
// same fix's negative case: a PATCH touching only name (or context/profile
// fields sent back unchanged) never increments config_revision.
func TestPatchCoordinator_PreservesConfigRevisionWhenFieldsUnchanged(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	newName := "Renamed"
	updated, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &newName}, nil)
	if err != nil {
		t.Fatalf("PatchCoordinator: %v", err)
	}
	if updated.ConfigRevision != 0 {
		t.Fatalf("updated.ConfigRevision = %d, want 0 (name-only change)", updated.ConfigRevision)
	}
}

// TestPatchCoordinator_ConcurrentDisjointFields is Build decision 7's test
// (a): two concurrent PATCHes on one coordinator touching disjoint fields
// must both apply, whichever order they commit in.
func TestPatchCoordinator_ConcurrentDisjointFields(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("rep-%d", i), func(t *testing.T) {
			store := newTestStore(t)
			ctx := context.Background()
			c := &Coordinator{WorkspaceID: "ws-1", Name: "orig", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig-ctx"}
			if err := store.CreateCoordinator(ctx, c); err != nil {
				t.Fatalf("CreateCoordinator: %v", err)
			}

			start := make(chan struct{})
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				name := "alpha"
				_, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
				errs <- err
			}()
			go func() {
				defer wg.Done()
				<-start
				newContext := "beta-ctx"
				_, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Context: &newContext}, nil)
				errs <- err
			}()
			close(start)
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("PatchCoordinator: %v", err)
				}
			}

			got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
			if err != nil {
				t.Fatalf("GetCoordinator: %v", err)
			}
			if got.Name != "alpha" || got.Context != "beta-ctx" {
				t.Fatalf("GetCoordinator after concurrent PATCH = %+v, want Name=alpha Context=beta-ctx", got)
			}
		})
	}
}

// TestPatchCoordinator_SameFieldDeterministicOrder is Build decision 7's test
// (b): PATCH A takes the lock and blocks in afterLock; PATCH B is started
// only once A is known to hold the lock, so B can never commit before A.
// With a strictly increasing fake clock, the winner's persisted state and
// UpdatedAt must be B's, and B's UpdatedAt must be strictly after A's.
func TestPatchCoordinator_SameFieldDeterministicOrder(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("rep-%d", i), func(t *testing.T) {
			store := newTestStore(t)
			ctx := context.Background()
			c := &Coordinator{WorkspaceID: "ws-1", Name: "orig", AgentProfileID: "a", ExecutorProfileID: "e"}
			if err := store.CreateCoordinator(ctx, c); err != nil {
				t.Fatalf("CreateCoordinator: %v", err)
			}

			var tick int64
			store.now = func() time.Time {
				n := atomic.AddInt64(&tick, 1)
				return time.Unix(0, n)
			}

			locked := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			// Block only the first afterLock invocation (A's): B also calls
			// this hook, but must run to completion once it gets the lock.
			store.afterLock = func(_ context.Context) {
				once.Do(func() {
					close(locked)
					<-release
				})
			}

			var wg sync.WaitGroup
			var aResult, bResult *Coordinator
			var aErr, bErr error

			wg.Add(1)
			go func() {
				defer wg.Done()
				name := "alpha"
				aResult, _, aErr = store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
			}()

			<-locked // A holds the write lock and is blocked inside afterLock.

			wg.Add(1)
			go func() {
				defer wg.Done()
				name := "beta"
				bResult, _, bErr = store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
			}()

			// B cannot pass the lock while A holds it, regardless of timing
			// from here: closing release only lets A proceed to commit.
			close(release)
			wg.Wait()

			if aErr != nil {
				t.Fatalf("PATCH A: %v", aErr)
			}
			if bErr != nil {
				t.Fatalf("PATCH B: %v", bErr)
			}

			got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
			if err != nil {
				t.Fatalf("GetCoordinator: %v", err)
			}
			if got.Name != "beta" {
				t.Fatalf("GetCoordinator.Name = %q, want %q (B commits second)", got.Name, "beta")
			}
			if !got.UpdatedAt.Equal(bResult.UpdatedAt) {
				t.Fatalf("GetCoordinator.UpdatedAt = %v, want B's returned %v", got.UpdatedAt, bResult.UpdatedAt)
			}
			if !bResult.UpdatedAt.After(aResult.UpdatedAt) {
				t.Fatalf("B.UpdatedAt = %v, want strictly after A.UpdatedAt = %v", bResult.UpdatedAt, aResult.UpdatedAt)
			}
		})
	}
}
