package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// fixedSizeASCIIContent returns content of exactly n bytes, built from ASCII
// so byte length and rune count agree — the multi-byte case is covered
// separately below.
func fixedSizeASCIIContent(n int) string {
	return strings.Repeat("a", n)
}

func TestPlanService_ContentSizeCeiling_Boundary(t *testing.T) {
	atCeiling := fixedSizeASCIIContent(MaxPlanContentBytes)
	overCeiling := fixedSizeASCIIContent(MaxPlanContentBytes + 1)

	t.Run("CreatePlan admits exactly the ceiling", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-at-ceiling-create")

		result, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-at-ceiling-create", Content: atCeiling})
		if err != nil {
			t.Fatalf("CreatePlan at ceiling: %v", err)
		}
		if len(result.Plan.Content) != MaxPlanContentBytes {
			t.Fatalf("stored content length = %d, want %d", len(result.Plan.Content), MaxPlanContentBytes)
		}
	})

	t.Run("CreatePlan rejects one byte over the ceiling", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-over-ceiling-create")

		_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-over-ceiling-create", Content: overCeiling})
		assertPlanContentTooLarge(t, err, MaxPlanContentBytes+1)
	})

	t.Run("UpdatePlan admits exactly the ceiling", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-at-ceiling-update")
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-at-ceiling-update", Content: "seed"}); err != nil {
			t.Fatalf("seed CreatePlan: %v", err)
		}

		result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-at-ceiling-update", Content: atCeiling})
		if err != nil {
			t.Fatalf("UpdatePlan at ceiling: %v", err)
		}
		if len(result.Plan.Content) != MaxPlanContentBytes {
			t.Fatalf("stored content length = %d, want %d", len(result.Plan.Content), MaxPlanContentBytes)
		}
	})

	t.Run("UpdatePlan rejects one byte over the ceiling", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-over-ceiling-update")
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-over-ceiling-update", Content: "seed"}); err != nil {
			t.Fatalf("seed CreatePlan: %v", err)
		}

		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-over-ceiling-update", Content: overCeiling})
		assertPlanContentTooLarge(t, err, MaxPlanContentBytes+1)
	})
}

// TestPlanService_ContentSizeCeiling_EmptyContentUnchanged pins the
// empty-content half of AC-001.8: empty content is at/below the ceiling and
// must be admitted exactly as it is today (the browser write path already
// accepts it; the MCP handlers reject it before the service is ever called).
func TestPlanService_ContentSizeCeiling_EmptyContentUnchanged(t *testing.T) {
	t.Run("CreatePlan admits empty content", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-empty-create")

		result, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-empty-create", Content: ""})
		if err != nil {
			t.Fatalf("CreatePlan with empty content: %v", err)
		}
		if result.Plan.Content != "" {
			t.Fatalf("stored content = %q, want empty", result.Plan.Content)
		}
	})

	t.Run("UpdatePlan admits empty content", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-empty-update")
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-empty-update", Content: "seed"}); err != nil {
			t.Fatalf("seed CreatePlan: %v", err)
		}

		result, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-empty-update", Content: ""})
		if err != nil {
			t.Fatalf("UpdatePlan with empty content: %v", err)
		}
		if result.Plan.Content != "" {
			t.Fatalf("stored content = %q, want empty", result.Plan.Content)
		}
	})
}

func assertPlanContentTooLarge(t *testing.T, err error, wantSubmitted int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrPlanContentTooLarge) {
		t.Fatalf("errors.Is(err, ErrPlanContentTooLarge) = false, err = %v", err)
	}
	var sizeErr *PlanContentTooLargeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("errors.As did not find *PlanContentTooLargeError in %v", err)
	}
	if sizeErr.Limit != MaxPlanContentBytes {
		t.Errorf("Limit = %d, want %d", sizeErr.Limit, MaxPlanContentBytes)
	}
	if sizeErr.Submitted != wantSubmitted {
		t.Errorf("Submitted = %d, want %d", sizeErr.Submitted, wantSubmitted)
	}
}

// TestPlanService_ContentSizeCeiling_RejectionPersistsNothing pins AC-001.4:
// a rejected write leaves HEAD unchanged, appends/coalesces no revision, and
// publishes no event.
func TestPlanService_ContentSizeCeiling_RejectionPersistsNothing(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	seeded, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Title: "Original", Content: "small content"})
	if err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}
	revisionsBefore, err := svc.ListRevisions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListRevisions before: %v", err)
	}
	eventBus.ClearEvents()

	_, err = svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: fixedSizeASCIIContent(MaxPlanContentBytes + 1)})
	assertPlanContentTooLarge(t, err, MaxPlanContentBytes+1)

	headAfter, err := svc.GetPlan(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetPlan after rejection: %v", err)
	}
	if headAfter.Content != seeded.Plan.Content {
		t.Fatalf("HEAD content changed: got %q, want %q", headAfter.Content, seeded.Plan.Content)
	}
	if headAfter.Title != seeded.Plan.Title {
		t.Fatalf("HEAD title changed: got %q, want %q", headAfter.Title, seeded.Plan.Title)
	}

	revisionsAfter, err := svc.ListRevisions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListRevisions after: %v", err)
	}
	if len(revisionsAfter) != len(revisionsBefore) {
		t.Fatalf("revision count changed: got %d, want %d", len(revisionsAfter), len(revisionsBefore))
	}

	if published := eventBus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("expected no events published on rejection, got %d: %+v", len(published), published)
	}
}

// TestPlanService_ContentSizeCeiling_Ordering pins AC-001.5: the task-id and
// access checks run before the size response, so an oversized write for a
// missing or inaccessible task does not expose the storage constraint. The
// size check still runs before plan storage and the per-task write lock.
func TestPlanService_ContentSizeCeiling_Ordering(t *testing.T) {
	oversized := fixedSizeASCIIContent(MaxPlanContentBytes + 1)

	t.Run("missing task_id takes precedence over an oversized write", func(t *testing.T) {
		svc, _, _ := createTestPlanService(t)
		_, err := svc.CreatePlan(context.Background(), CreatePlanRequest{Content: oversized})
		if !errors.Is(err, ErrTaskIDRequired) {
			t.Fatalf("expected ErrTaskIDRequired, got %v", err)
		}
	})

	t.Run("missing task takes precedence over an oversized write", func(t *testing.T) {
		svc, _, _ := createTestPlanService(t)

		_, err := svc.CreatePlan(context.Background(), CreatePlanRequest{
			TaskID:  "missing-task",
			Content: oversized,
		})
		if !errors.Is(err, repoerrors.ErrTaskNotFound) {
			t.Fatalf("expected repoerrors.ErrTaskNotFound, got %v", err)
		}
		if errors.Is(err, ErrPlanContentTooLarge) {
			t.Fatalf("missing task must not expose the size constraint: %v", err)
		}
	})

	t.Run("UpdatePlan also preserves missing task precedence", func(t *testing.T) {
		svc, _, _ := createTestPlanService(t)

		_, err := svc.UpdatePlan(context.Background(), UpdatePlanRequest{
			TaskID:  "missing-task",
			Content: oversized,
		})
		if !errors.Is(err, repoerrors.ErrTaskNotFound) {
			t.Fatalf("expected repoerrors.ErrTaskNotFound, got %v", err)
		}
		if errors.Is(err, ErrPlanContentTooLarge) {
			t.Fatalf("missing task must not expose the size constraint: %v", err)
		}
	})

	t.Run("authorization runs before the size response", func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-1")

		authorizerCalled := false
		svc.SetTaskAuthorizer(func(ctx context.Context, taskID string) error {
			authorizerCalled = true
			return errors.New("caller cannot access this task")
		})

		_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: oversized})
		if err == nil || err.Error() != "caller cannot access this task" {
			t.Fatalf("expected authorization error, got %v", err)
		}
		if !authorizerCalled {
			t.Fatal("authorizer was not called before evaluating an oversized write")
		}
	})
}

// TestPlanService_ContentSizeCeiling_NoLockContention pins the second half of
// AC-001.5: an oversized write must not wait behind another write's per-task
// lock, because the admission check reads no plan state and needs no
// serialization.
// Uses synctest so the test remains deterministic without a wall-clock wait.
func TestPlanService_ContentSizeCeiling_NoLockContention(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, repo := createTestPlanService(t)
		ctx := context.Background()
		seedTask(t, ctx, repo, "task-1")

		release := svc.locks.acquire("task-1")
		defer release()

		done := make(chan error, 1)
		go func() {
			_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: fixedSizeASCIIContent(MaxPlanContentBytes + 1)})
			done <- err
		}()

		synctest.Wait()
		select {
		case err := <-done:
			assertPlanContentTooLarge(t, err, MaxPlanContentBytes+1)
		default:
			t.Fatal("oversized write blocked behind the task's write lock")
		}
	})
}

// TestPlanService_ContentSizeCeiling_RevertExempt pins AC-001.9: RevertPlan
// restores an above-ceiling stored revision successfully.
func TestPlanService_ContentSizeCeiling_RevertExempt(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	// Seed an above-ceiling revision directly through the repository, as if
	// it were written before this ceiling existed.
	oversized := fixedSizeASCIIContent(MaxPlanContentBytes + 5000)
	plan := &models.TaskPlan{TaskID: "task-1", Title: "Legacy", Content: oversized, CreatedBy: "user"}
	rev := &models.TaskPlanRevision{TaskID: "task-1", Title: "Legacy", Content: oversized, AuthorKind: "user", AuthorName: "User"}
	if err := repo.WritePlanRevision(ctx, plan, rev, nil, false, false); err != nil {
		t.Fatalf("seed oversized revision: %v", err)
	}

	// AC-001.10: an above-ceiling revision must still be listable in revision
	// history, not just revertable.
	revisions, err := svc.ListRevisions(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListRevisions with an above-ceiling revision: %v", err)
	}
	found := false
	for _, r := range revisions {
		if r.ID == rev.ID {
			found = true
			if len(r.Content) != len(oversized) {
				t.Fatalf("listed revision content length = %d, want %d", len(r.Content), len(oversized))
			}
		}
	}
	if !found {
		t.Fatalf("above-ceiling revision %s not present in ListRevisions", rev.ID)
	}

	// A subsequent normal-sized write becomes HEAD; the oversized row survives
	// only in revision history, which is what this test reverts back to.
	// CreatedBy is explicitly "agent" (mismatching the seeded revision's
	// "user" author) so this write appends a new revision instead of
	// coalescing into and overwriting the seeded one.
	if _, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: "current small plan", CreatedBy: "agent"}); err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}

	reverted, err := svc.RevertPlan(ctx, RevertPlanRequest{TaskID: "task-1", TargetRevisionID: rev.ID})
	if err != nil {
		t.Fatalf("RevertPlan to an above-ceiling revision: %v", err)
	}
	if reverted.Content != oversized {
		t.Fatalf("reverted revision content length = %d, want %d", len(reverted.Content), len(oversized))
	}

	head, err := svc.GetPlan(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetPlan after revert: %v", err)
	}
	if head.Content != oversized {
		t.Fatalf("HEAD content length after revert = %d, want %d", len(head.Content), len(oversized))
	}
}

// TestPlanService_ContentSizeCeiling_MultiByteBoundary pins the byte-vs-rune
// unit choice (system design "The constant and the unit"): a document whose
// byte length sits just under the ceiling, but whose rune count is far
// lower, is admitted.
func TestPlanService_ContentSizeCeiling_MultiByteBoundary(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	// "日" is 3 bytes in UTF-8. Build a document whose byte length sits just
	// under the ceiling but whose rune count (a third of that) is far lower —
	// a rune-counting implementation would admit it too, but so would one
	// that undercounts multi-byte content as runes-only; the companion
	// rejection case below is what actually discriminates the unit.
	rune3 := "日"
	repeats := MaxPlanContentBytes / len([]byte(rune3))
	content := strings.Repeat(rune3, repeats)
	if len(content) > MaxPlanContentBytes {
		t.Fatalf("test setup: content is %d bytes, want <= %d", len(content), MaxPlanContentBytes)
	}
	if len([]rune(content)) >= len(content) {
		t.Fatalf("test setup: rune count %d is not lower than the byte count %d", len([]rune(content)), len(content))
	}

	result, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: content})
	if err != nil {
		t.Fatalf("CreatePlan with a multi-byte document under the byte ceiling: %v", err)
	}
	if result.Plan.Content != content {
		t.Fatal("stored content does not match submitted multi-byte content")
	}
}

// TestPlanService_ContentSizeCeiling_ConcurrentWritesIndependent pins
// AC-001.6: the ceiling decides from the submitted content alone, so two
// concurrent writes for the same task — one over the ceiling, one at or
// under it — each get the outcome their own content deserves, regardless of
// interleaving or of which one the other's rejection might otherwise seem to
// influence.
func TestPlanService_ContentSizeCeiling_ConcurrentWritesIndependent(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: "seed"}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	oversizedErr := make(chan error, 1)
	var admittedErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: fixedSizeASCIIContent(MaxPlanContentBytes + 1)})
		oversizedErr <- err
	}()
	go func() {
		defer wg.Done()
		_, admittedErr = svc.UpdatePlan(ctx, UpdatePlanRequest{TaskID: "task-1", Content: "small admitted content"})
	}()
	wg.Wait()
	close(oversizedErr)

	assertPlanContentTooLarge(t, <-oversizedErr, MaxPlanContentBytes+1)
	if admittedErr != nil {
		t.Fatalf("admitted write returned an error: %v", admittedErr)
	}

	head, err := svc.GetPlan(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if head.Content != "small admitted content" {
		t.Fatalf("HEAD content = %q, want the admitted write's content", head.Content)
	}
}

// TestPlanService_ContentSizeCeiling_ResubmissionRetainsNoState pins
// AC-001.7: resubmitting content that was just rejected is judged fresh —
// the same rejection, not a different one, and not an escalating one.
func TestPlanService_ContentSizeCeiling_ResubmissionRetainsNoState(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")
	oversized := fixedSizeASCIIContent(MaxPlanContentBytes + 1)

	_, firstErr := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: oversized})
	assertPlanContentTooLarge(t, firstErr, MaxPlanContentBytes+1)

	_, secondErr := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: oversized})
	assertPlanContentTooLarge(t, secondErr, MaxPlanContentBytes+1)

	if firstErr.Error() != secondErr.Error() {
		t.Fatalf("resubmission produced a different message:\nfirst:  %s\nsecond: %s", firstErr, secondErr)
	}

	// The task never had a plan created (both calls were rejected before
	// storage), so a later, admitted create must still succeed — nothing
	// from the earlier rejections was retained against this task.
	if _, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: "now under the ceiling"}); err != nil {
		t.Fatalf("CreatePlan after two rejections: %v", err)
	}
}

// TestPlanService_ContentSizeCeiling_MultiByteRejection is the discriminating
// half of the byte-vs-rune pin: a document whose RUNE count is at the
// ceiling but whose BYTE length is nearly 3x over it must still be rejected.
// A rune-counting implementation would wrongly admit this.
func TestPlanService_ContentSizeCeiling_MultiByteRejection(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	rune3 := "日"
	content := strings.Repeat(rune3, MaxPlanContentBytes)
	if len([]rune(content)) != MaxPlanContentBytes {
		t.Fatalf("test setup: rune count = %d, want %d", len([]rune(content)), MaxPlanContentBytes)
	}
	if len(content) <= MaxPlanContentBytes {
		t.Fatalf("test setup: byte length %d is not over the ceiling", len(content))
	}

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: "task-1", Content: content})
	assertPlanContentTooLarge(t, err, len(content))
}

// TestPlanContentTooLargeError_MessageStatesEveryRequiredFact pins
// AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.2 and -002.3: the message is the
// agent's only signal, so its wording — not just its two numbers — is the
// contract. It must state that nothing was stored and the existing plan is
// unchanged (002.2), and it must explicitly direct the caller to shorten the
// document it holds rather than retrying the same content unchanged or
// reconstructing the plan from memory (002.3) — the prohibition has to be
// stated, not merely have its topic absent, so this checks for the guidance
// phrases rather than for the bare absence of "retry"/"reconstruct".
func TestPlanContentTooLargeError_MessageStatesEveryRequiredFact(t *testing.T) {
	err := &PlanContentTooLargeError{Submitted: MaxPlanContentBytes + 1, Limit: MaxPlanContentBytes}
	msg := strings.ToLower(err.Error())

	requiredPhrases := []string{
		"nothing was stored",                      // AC-002.2: nothing was stored
		"unchanged",                               // AC-002.2: existing plan is unchanged
		"shorten the document",                    // AC-002.3: directs to shorten the held document
		"do not resubmit this content",            // AC-002.3: does not instruct retrying unchanged
		"do not reconstruct the plan from memory", // AC-002.3: does not suggest reconstructing from memory
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(msg, phrase) {
			t.Errorf("message %q does not contain required phrase %q", msg, phrase)
		}
	}
}
