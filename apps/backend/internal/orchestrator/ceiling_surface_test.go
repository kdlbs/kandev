package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// TestDeferCeilingRefusal_WritesCardNoteOnFirstRefusal pins AC-49: a
// session-bearing refusal writes the card note inline and stamps
// ceiling_surface_written_at so a later attempt does not repeat it.
func TestDeferCeilingRefusal_WritesCardNoteOnFirstRefusal(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "surface-task", "surface-session", models.TaskSessionStateRunning)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	require.NoError(t, svc.deferCeilingRefusal(
		ctx, "surface-task", "surface-session", models.CeilingLaunchResume,
		map[string]interface{}{metaKeySessionID: "surface-session"}, ceilingReasonRefused, 3, true, 3,
	))

	require.Len(t, messages.sessionMessages, 1)
	note := messages.sessionMessages[0]
	require.Equal(t, "surface-session", note.sessionID)
	require.Equal(t, metaVariantCeiling, note.metadata[metaKeyVariant])
	require.Equal(t, ceilingReasonRefused, note.metadata[ceilingFieldReasonCode])

	record := deferredLaunchOf(t, svc, "surface-task")
	require.NotEmpty(t, record[models.CeilingSurfaceWrittenAtKey], "the success stamp must be recorded")
}

// TestDeferCeilingRefusal_SeamOneNeverWritesACardNote pins AC-49's exclusion
// of seam 1: a "start" kind refusal carries no session id, so it must never
// reach the carrier at all.
func TestDeferCeilingRefusal_SeamOneNeverWritesACardNote(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "surface-seam1-task", Title: "T"}))
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	require.NoError(t, svc.deferCeilingRefusal(
		ctx, "surface-seam1-task", "", models.CeilingLaunchStart,
		map[string]interface{}{"prompt": "p"}, ceilingReasonRefused, 3, true, 3,
	))

	require.Empty(t, messages.sessionMessages)
	record := deferredLaunchOf(t, svc, "surface-seam1-task")
	require.Nil(t, record[models.CeilingSurfaceWrittenAtKey])
}

// TestDeferCeilingRefusal_SameReasonDoesNotRewriteTheNote pins AC-49e:
// suppression is keyed on a note having been successfully written, so a
// repeat refusal for the same unchanged reason must not write a second one.
func TestDeferCeilingRefusal_SameReasonDoesNotRewriteTheNote(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "surface-dup-task", "surface-dup-session", models.TaskSessionStateRunning)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	payload := map[string]interface{}{metaKeySessionID: "surface-dup-session"}
	require.NoError(t, svc.deferCeilingRefusal(ctx, "surface-dup-task", "surface-dup-session", models.CeilingLaunchResume, payload, ceilingReasonRefused, 3, true, 3))
	require.NoError(t, svc.deferCeilingRefusal(ctx, "surface-dup-task", "surface-dup-session", models.CeilingLaunchResume, payload, ceilingReasonRefused, 3, true, 3))

	require.Len(t, messages.sessionMessages, 1, "an unchanged reason must not write a second note")
}

func TestWriteCeilingSurfaceNote_IsIdentityGuardedAndIdempotent(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "surface-identity-task", "surface-identity-session", models.TaskSessionStateRunning)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	old := models.CeilingDeferral{
		Kind: models.CeilingLaunchResume, Origin: string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused, QueuedAt: time.Now().UTC().Truncate(time.Second), Ceiling: 3,
		Payload: map[string]interface{}{metaKeySessionID: "surface-identity-session"},
	}
	newer := old
	newer.ReasonCode = ceilingReasonUnknownPopulation
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "surface-identity-task", models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(newer)))
	require.ErrorIs(t, svc.writeCeilingSurfaceNote(ctx, "surface-identity-task", "surface-identity-session", old), errCeilingSurfaceSuperseded)
	require.Empty(t, messages.sessionMessages)

	require.NoError(t, repo.SetTaskMetadataKey(ctx, "surface-identity-task", models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(old)))
	require.NoError(t, svc.writeCeilingSurfaceNote(ctx, "surface-identity-task", "surface-identity-session", old))
	require.NoError(t, svc.writeCeilingSurfaceNote(ctx, "surface-identity-task", "surface-identity-session", old))
	require.Len(t, messages.sessionMessages, 1)
}

// TestDeferCeilingRefusal_ReasonChangeRewritesTheRecordAndTheNote pins AC-49f:
// the same pending launch refused again for a different reason updates the
// stored reason code and gets its own note.
func TestDeferCeilingRefusal_ReasonChangeRewritesTheRecordAndTheNote(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "surface-change-task", "surface-change-session", models.TaskSessionStateRunning)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	payload := map[string]interface{}{metaKeySessionID: "surface-change-session"}
	require.NoError(t, svc.deferCeilingRefusal(ctx, "surface-change-task", "surface-change-session", models.CeilingLaunchResume, payload, ceilingReasonUnknownPopulation, 0, false, 3))
	require.NoError(t, svc.deferCeilingRefusal(ctx, "surface-change-task", "surface-change-session", models.CeilingLaunchResume, payload, ceilingReasonRefused, 3, true, 3))

	require.Len(t, messages.sessionMessages, 2, "a reason-code change must write its own note")
	require.Equal(t, ceilingReasonUnknownPopulation, messages.sessionMessages[0].metadata[ceilingFieldReasonCode])
	require.Equal(t, ceilingReasonRefused, messages.sessionMessages[1].metadata[ceilingFieldReasonCode])

	record := deferredLaunchOf(t, svc, "surface-change-task")
	require.Equal(t, ceilingReasonRefused, record[models.CeilingReasonCodeKey])
	require.NotEmpty(t, record[models.CeilingSurfaceWrittenAtKey])
}

// TestAttemptCeilingSurfaceWrite_RetriesUpToBudgetThenStops pins
// AC-49g/AC-49g1: a carrier write that keeps failing is retried on each
// sweep pass, bounded at five consecutive failures, after which no further
// attempt is made.
func TestAttemptCeilingSurfaceWrite_RetriesUpToBudgetThenStops(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "surface-fail-task", "surface-fail-session", models.TaskSessionStateRunning)
	failing := &mockMessageCreator{sessionMessageErr: errors.New("carrier down")}
	svc.messageCreator = failing

	require.NoError(t, svc.deferCeilingRefusal(
		ctx, "surface-fail-task", "surface-fail-session", models.CeilingLaunchResume,
		map[string]interface{}{metaKeySessionID: "surface-fail-session"}, ceilingReasonRefused, 3, true, 3,
	))
	require.Equal(t, 1, failing.sessionMessageAttempts)

	// Four more sweep-driven retries reach the five-attempt budget.
	for i := 0; i < 4; i++ {
		svc.attemptCeilingSurfaceWrite(ctx, "surface-fail-task")
	}
	require.Equal(t, 5, failing.sessionMessageAttempts)

	// The budget is exhausted: a further tick must not attempt again.
	svc.attemptCeilingSurfaceWrite(ctx, "surface-fail-task")
	require.Equal(t, 5, failing.sessionMessageAttempts, "must stop attempting once the budget is exhausted")

	record := deferredLaunchOf(t, svc, "surface-fail-task")
	require.Nil(t, record[models.CeilingSurfaceWrittenAtKey])
	require.EqualValues(t, 5, record[models.CeilingSurfaceAttemptCountKey])
}

// TestAttemptCeilingSurfaceWrite_SucceedsAfterEarlierFailuresStampsWritten
// covers the recovery path: once the carrier starts working again, the next
// attempt succeeds, stamps ceiling_surface_written_at and clears the failure
// counter.
func TestAttemptCeilingSurfaceWrite_SucceedsAfterEarlierFailuresStampsWritten(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "surface-recover-task", "surface-recover-session", models.TaskSessionStateRunning)
	messages := &mockMessageCreator{sessionMessageErr: errors.New("carrier down")}
	svc.messageCreator = messages

	require.NoError(t, svc.deferCeilingRefusal(
		ctx, "surface-recover-task", "surface-recover-session", models.CeilingLaunchResume,
		map[string]interface{}{metaKeySessionID: "surface-recover-session"}, ceilingReasonRefused, 3, true, 3,
	))
	record := deferredLaunchOf(t, svc, "surface-recover-task")
	require.EqualValues(t, 1, record[models.CeilingSurfaceAttemptCountKey])

	messages.sessionMessageErr = nil
	svc.attemptCeilingSurfaceWrite(ctx, "surface-recover-task")

	require.Len(t, messages.sessionMessages, 1)
	record = deferredLaunchOf(t, svc, "surface-recover-task")
	require.NotEmpty(t, record[models.CeilingSurfaceWrittenAtKey])
	require.Nil(t, record[models.CeilingSurfaceAttemptCountKey])
}
