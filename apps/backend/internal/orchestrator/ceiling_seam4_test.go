package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTransientLaunchRetryUsesAutomaticCeilingOrigin(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"retry-ceiling-first", "retry-ceiling-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam4(ctx, "retry-ceiling-first", "retry-ceiling-first-session", launchOriginAutomatic, map[string]interface{}{}); err != nil || deferred {
		t.Fatalf("first automatic resume was not admitted: deferred=%v err=%v", deferred, err)
	}

	identity := transientLaunchFailureIdentity{
		attemptID:  "retry-attempt",
		errorStamp: "retry-error",
		updatedAt:  time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC),
	}
	request := &LaunchSessionRequest{Origin: string(launchOriginAutomatic)}
	origin := launchOriginForActivation(request)
	options := executor.ResumeOptions{
		Origin:                   string(origin),
		TransientRetryAttemptID:  identity.attemptID,
		TransientRetryErrorStamp: identity.errorStamp,
		TransientRetryUpdatedAt:  identity.updatedAt,
	}
	_, deferred, err := svc.admitOrDeferSeam4(ctx, "retry-ceiling-second", "retry-ceiling-second-session", origin,
		seam4ResumePayload("retry-ceiling-second-session", options))
	if err != nil {
		t.Fatalf("admitOrDeferSeam4: %v", err)
	}
	if !deferred {
		t.Fatal("an automatic retry must defer when the session ceiling is full")
	}
	deferredRecord, err := models.ReadCeilingDeferral(deferredLaunchOf(t, svc, "retry-ceiling-second"))
	if err != nil {
		t.Fatalf("ReadCeilingDeferral: %v", err)
	}
	if deferredRecord.Origin != string(launchOriginAutomatic) {
		t.Fatalf("deferred retry origin = %q, want automatic", deferredRecord.Origin)
	}
	got, found, err := transientLaunchIdentityFromPayload(deferredRecord.Payload)
	if err != nil || !found || got != identity {
		t.Fatalf("deferred retry identity = %+v, found=%v err=%v; want %+v", got, found, err, identity)
	}
}

func TestSeam4ResumePayloadKeepsTransientRetryIdentity(t *testing.T) {
	want := transientLaunchFailureIdentity{
		attemptID:  "failed-start-attempt",
		errorStamp: "failed-start-stamp",
		updatedAt:  time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC),
	}
	payload := seam4ResumePayload("retry-session", executor.ResumeOptions{
		TransientRetryAttemptID:  want.attemptID,
		TransientRetryErrorStamp: want.errorStamp,
		TransientRetryUpdatedAt:  want.updatedAt,
	})
	got, found, err := transientLaunchIdentityFromPayload(payload)
	if err != nil || !found || got != want {
		t.Fatalf("transient retry identity round trip = %+v, found=%v err=%v; want %+v", got, found, err, want)
	}
}

// TestAdmitOrDeferSeam4AdmitsUnderCeiling covers the ordinary case: population
// below the ceiling, admitted, no record written, reservation keyed by the
// session being resumed.
func TestAdmitOrDeferSeam4AdmitsUnderCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 5)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam4-admit", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-admit", "seam4-admit-session", launchOriginAutomatic, map[string]interface{}{"session_id": "seam4-admit-session"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam4: %v", err)
	}
	if deferred {
		t.Fatal("resume was deferred while under the ceiling")
	}
	if reservation == nil || reservation.key != "seam4-admit-session" {
		t.Fatalf("reservation not keyed by the session being resumed: %+v", reservation)
	}

	record := deferredLaunchOf(t, svc, "seam4-admit")
	if record != nil {
		t.Fatalf("a deferred_launch record was written for an admitted resume: %+v", record)
	}
}

// TestAdmitOrDeferSeam4DefersAutomaticOverCeiling covers AC-11/AC-42: an
// automatic resume at the ceiling is refused and its "resume" payload is
// recorded, not dropped.
func TestAdmitOrDeferSeam4DefersAutomaticOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam4-first", "seam4-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	if _, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-first", "seam4-first-session", launchOriginAutomatic, map[string]interface{}{"session_id": "seam4-first-session"}); err != nil || deferred {
		t.Fatalf("first resume was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-second", "seam4-second-session", launchOriginAutomatic, seam4ResumePayload("seam4-second-session", executor.ResumeOptions{AllowBranchReplacement: true}))
	if err != nil {
		t.Fatalf("admitOrDeferSeam4: %v", err)
	}
	if !deferred {
		t.Fatal("second automatic resume over the ceiling was admitted, want deferred")
	}
	if reservation != nil {
		t.Fatal("a deferred resume must not hold a reservation")
	}

	record := deferredLaunchOf(t, svc, "seam4-second")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the refused resume's payload was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchResume) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchResume)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["allow_branch_replacement"] != true {
		t.Fatalf("the recorded payload does not match the refused resume: %+v", record)
	}
}

// TestAdmitOrDeferSeam4AdmitsManualOverCeiling covers AC-14: a manual resume is
// always admitted, never deferred, even at the ceiling.
func TestAdmitOrDeferSeam4AdmitsManualOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam4-manual-first", "seam4-manual-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-manual-first", "seam4-manual-first-session", launchOriginAutomatic, map[string]interface{}{"session_id": "seam4-manual-first-session"}); err != nil || deferred {
		t.Fatalf("first resume was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-manual-second", "seam4-manual-second-session", launchOriginManual, map[string]interface{}{"session_id": "seam4-manual-second-session"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam4: %v", err)
	}
	if deferred {
		t.Fatal("a manual resume was deferred; AC-14 requires it always be admitted")
	}
	if reservation == nil {
		t.Fatal("a manual override still consumes a reservation")
	}
	if record := deferredLaunchOf(t, svc, "seam4-manual-second"); record != nil {
		t.Fatalf("a manual override must never write a ceiling_deferred record: %+v", record)
	}
}

// TestAdmitOrDeferSeam4ReportsWriteFailure covers AC-45: a refusal that cannot
// be persisted must not be swallowed.
func TestAdmitOrDeferSeam4ReportsWriteFailure(t *testing.T) {
	svc, _ := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "resumeTaskSession"})

	_, _, err := svc.admitOrDeferSeam4(ctx, "missing-task", "missing-session", launchOriginAutomatic, map[string]interface{}{"session_id": "missing-session"})
	if err == nil {
		t.Fatal("expected an error when the refusal could not be persisted")
	}
}

// TestAdmitOrDeferSeam4UnsetOriginDefaultsAutomatic pins AC-13b directly
// against the controller: an unset/invalid origin string (what ResumeOptions's
// zero value converts to) must classify as automatic, not fall through as an
// unconditional manual admit.
func TestAdmitOrDeferSeam4UnsetOriginDefaultsAutomatic(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam4-unset-first", "seam4-unset-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-unset-first", "seam4-unset-first-session", launchOriginAutomatic, map[string]interface{}{}); err != nil || deferred {
		t.Fatalf("first resume was not admitted: deferred=%v err=%v", deferred, err)
	}

	// launchOrigin("") is neither "manual" nor "automatic".
	_, deferred, err := svc.admitOrDeferSeam4(ctx, "seam4-unset-second", "seam4-unset-second-session", launchOrigin(""), map[string]interface{}{})
	if err != nil {
		t.Fatalf("admitOrDeferSeam4: %v", err)
	}
	if !deferred {
		t.Fatal("an unset origin over the ceiling must default to automatic and be deferred, not admitted as manual")
	}
}
