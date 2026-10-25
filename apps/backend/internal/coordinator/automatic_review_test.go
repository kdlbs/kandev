package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func countReviews(t *testing.T, store *Store, c *Coordinator) int {
	t.Helper()
	var n int
	if err := store.db.GetContext(context.Background(), &n, store.db.Rebind(`SELECT COUNT(*) FROM coordinator_class_reviews WHERE coordinator_id = ?`), c.ID); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestClassReview_ServerComputesEveryField(t *testing.T) {
	store, c, _, svc, log := automaticFixture(t)
	mustExec(t, store, `DELETE FROM coordinator_class_reviews`)
	log.rows = decisions(23, 20)
	review, err := svc.RecordClassReview(authedContext("mgr-1"), c.WorkspaceID, c.ID, "create_task")
	if err != nil {
		t.Fatal(err)
	}
	if review.ReviewedBy != "mgr-1" || !review.ReviewedAt.Equal(automaticNow) || !review.WindowEnd.Equal(automaticNow) ||
		!review.WindowStart.Equal(automaticNow.Add(-30*24*time.Hour)) || review.RowCount != 23 {
		t.Fatalf("review = %+v", review)
	}
	stored, err := store.NewestClassReview(context.Background(), c.ID, ActionCreateTask)
	if err != nil || stored == nil || stored.RowCount != 23 || stored.ReviewedBy != "mgr-1" {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
}

func TestClassReview_LogErrorStoresNothing(t *testing.T) {
	store, c, _, svc, log := automaticFixture(t)
	before := countReviews(t, store, c)
	log.err = errors.New("boom")
	_, err := svc.RecordClassReview(context.Background(), c.WorkspaceID, c.ID, "create_task")
	var unavailable *DecisionLogUnavailableError
	if !errors.As(err, &unavailable) || countReviews(t, store, c) != before {
		t.Fatalf("err = %v", err)
	}
}

func TestClassReview_RefusesReaderOtherClassAndPhase3Off(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	before := countReviews(t, store, c)
	var classErr *ClassError
	if _, err := svc.RecordClassReview(context.Background(), c.WorkspaceID, c.ID, "merge"); !errors.As(err, &classErr) || classErr.Class != "merge" {
		t.Fatalf("err = %v", err)
	}
	svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}
	if _, err := svc.RecordClassReview(context.Background(), c.WorkspaceID, c.ID, "create_task"); !errors.Is(err, taskservice.ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	svc.authz = &fakeWorkspaceAuthorizer{}
	svc.phase3 = false
	if _, err := svc.RecordClassReview(context.Background(), c.WorkspaceID, c.ID, "create_task"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if countReviews(t, store, c) != before {
		t.Fatal("refused review stored a row")
	}
}

func TestGetEligibility_ReaderMayReadAndLogErrorIs503Shape(t *testing.T) {
	_, c, _, svc, log := automaticFixture(t)
	view, err := svc.GetEligibility(context.Background(), c.WorkspaceID, c.ID, "create_task")
	if err != nil || !view.Result.Eligible || view.Setting != SettingRequiresApproval {
		t.Fatalf("view = %+v err = %v", view, err)
	}
	var classErr *ClassError
	if _, err := svc.GetEligibility(context.Background(), c.WorkspaceID, c.ID, "move"); !errors.As(err, &classErr) {
		t.Fatalf("err = %v", err)
	}
	log.err = errors.New("boom")
	var unavailable *DecisionLogUnavailableError
	if _, err := svc.GetEligibility(context.Background(), c.WorkspaceID, c.ID, "create_task"); !errors.As(err, &unavailable) {
		t.Fatalf("err = %v", err)
	}
}
