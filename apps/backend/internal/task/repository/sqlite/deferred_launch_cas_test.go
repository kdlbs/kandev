package sqlite

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func newCeilingRecord(queuedAt int64) map[string]interface{} {
	return map[string]interface{}{
		models.CeilingDeferredKey:   true,
		models.CeilingQueuedAtKey:   queuedAt,
		models.CeilingLaunchKindKey: "start_task",
	}
}

func readTaskMetadata(t *testing.T, repo *Repository, taskID string) map[string]interface{} {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", taskID, err)
	}
	return task.Metadata
}

// TestSetTaskDeferredLaunchCreatesFromAbsent covers AC-40's new part: absent is a
// legal expected prior state, so the first writer can create the record.
func TestSetTaskDeferredLaunchCreatesFromAbsent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "cas-create", Title: "Create"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	record, prior, err := repo.GetTaskDeferredLaunch(ctx, "cas-create")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	if record != nil {
		t.Fatalf("GetTaskDeferredLaunch returned %v, want nil for a task with no record", record)
	}

	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-create", prior, newCeilingRecord(1757606400))
	if err != nil {
		t.Fatalf("SetTaskDeferredLaunchIfUnchanged: %v", err)
	}
	if !stored || lost {
		t.Fatalf("SetTaskDeferredLaunchIfUnchanged = (stored=%v, lost=%v), want (true, false)", stored, lost)
	}

	metadata := readTaskMetadata(t, repo, "cas-create")
	written, ok := metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("deferred_launch not written: %+v", metadata)
	}
	if written[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling_deferred not set: %+v", written)
	}
}

// TestSetTaskDeferredLaunchReportsLostCompareDistinguishably covers AC-40b: an
// expected prior of absent against a present record is a lost comparison, not an
// error, so the caller enters AC-12e's re-read path instead of AC-45's failure path.
func TestSetTaskDeferredLaunchReportsLostCompareDistinguishably(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "cas-lost", Title: "Lost"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, prior, err := repo.GetTaskDeferredLaunch(ctx, "cas-lost")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	// Another writer lands first, using the same absent prior.
	if stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-lost", prior, newCeilingRecord(111)); err != nil || !stored || lost {
		t.Fatalf("first write = (%v, %v, %v), want (true, false, nil)", stored, lost, err)
	}

	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-lost", prior, newCeilingRecord(222))
	if err != nil {
		t.Fatalf("lost compare reported as an error: %v", err)
	}
	if stored || !lost {
		t.Fatalf("second write = (stored=%v, lost=%v), want (false, true)", stored, lost)
	}

	written := readTaskMetadata(t, repo, "cas-lost")[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if got := jsonNumberOf(t, written[models.CeilingQueuedAtKey]); got != "111" {
		t.Fatalf("losing writer overwrote the stored record: ceiling_queued_at = %s, want 111", got)
	}
}

// TestSetTaskDeferredLaunchAcceptsAMatchingPresentPrior and its stale twin cover
// AC-40a: the comparison is over the whole deferred_launch value.
func TestSetTaskDeferredLaunchAcceptsAMatchingPresentPrior(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "cas-present", Title: "Present",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: newCeilingRecord(1757606400)},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	record, prior, err := repo.GetTaskDeferredLaunch(ctx, "cas-present")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("GetTaskDeferredLaunch returned %+v, want the stored ceiling record", record)
	}
	// A large integer must survive the read/compare round trip; a float64
	// round trip would render it in exponent form and the CAS could never win.
	if got := jsonNumberOf(t, record[models.CeilingQueuedAtKey]); got != "1757606400" {
		t.Fatalf("ceiling_queued_at read back as %s, want 1757606400", got)
	}

	record[models.CeilingReasonCodeKey] = "ceiling"
	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-present", prior, record)
	if err != nil || !stored || lost {
		t.Fatalf("SetTaskDeferredLaunchIfUnchanged = (%v, %v, %v), want (true, false, nil)", stored, lost, err)
	}
}

func TestSetTaskDeferredLaunchRejectsAStalePresentPrior(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "cas-stale", Title: "Stale",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: newCeilingRecord(1)},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	record, prior, err := repo.GetTaskDeferredLaunch(ctx, "cas-stale")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	// Somebody else advances the record after our read.
	_, newPrior, err := repo.GetTaskDeferredLaunch(ctx, "cas-stale")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	if stored, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-stale", newPrior, newCeilingRecord(2)); err != nil || !stored {
		t.Fatalf("interleaved write failed: stored=%v err=%v", stored, err)
	}

	record[models.CeilingReasonCodeKey] = "ceiling"
	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-stale", prior, record)
	if err != nil {
		t.Fatalf("stale compare reported as an error: %v", err)
	}
	if stored || !lost {
		t.Fatalf("stale write = (stored=%v, lost=%v), want (false, true)", stored, lost)
	}
}

// TestSetTaskDeferredLaunchLeavesOtherMetadataKeysAlone pins that the CAS writes
// one key: it must not become a whole-metadata replace.
func TestSetTaskDeferredLaunchLeavesOtherMetadataKeysAlone(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "cas-neighbours", Title: "Neighbours",
		Metadata: map[string]interface{}{"unrelated_key": "keep me"},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, prior, err := repo.GetTaskDeferredLaunch(ctx, "cas-neighbours")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	if stored, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-neighbours", prior, newCeilingRecord(5)); err != nil || !stored {
		t.Fatalf("write failed: stored=%v err=%v", stored, err)
	}

	metadata := readTaskMetadata(t, repo, "cas-neighbours")
	if metadata["unrelated_key"] != "keep me" {
		t.Fatalf("CAS clobbered a neighbouring metadata key: %+v", metadata)
	}
}

// TestSetTaskDeferredLaunchReportsMissingTaskAsAnError keeps a genuine failure
// distinguishable from AC-40b's expected lost comparison.
func TestSetTaskDeferredLaunchReportsMissingTaskAsAnError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if _, _, err := repo.GetTaskDeferredLaunch(ctx, "cas-missing"); err == nil {
		t.Fatal("GetTaskDeferredLaunch accepted a task that does not exist")
	}
	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-missing", AbsentDeferredLaunch(), newCeilingRecord(1))
	if err == nil {
		t.Fatalf("SetTaskDeferredLaunchIfUnchanged = (%v, %v, nil) for a missing task, want an error", stored, lost)
	}
	if lost {
		t.Fatal("a missing task was reported as a lost comparison")
	}
}

// TestSetTaskDeferredLaunchAdmitsExactlyOneConcurrentCreate is AC-12e's race: two
// automatic launches refused for the same task at the same moment both read "no
// record". Exactly one may store; the other must report a lost compare, not an
// error, and must not lose the stored payload.
func TestSetTaskDeferredLaunchAdmitsExactlyOneConcurrentCreate(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "cas-race", Title: "Race"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	const writers = 8
	var (
		wg       sync.WaitGroup
		readAll  sync.WaitGroup
		mu       sync.Mutex
		stores   int
		losses   int
		failures []error
	)
	// Every writer must hold its prior BEFORE any write lands, or the race
	// under test does not happen: a writer that reads after an earlier commit
	// holds a present prior, and its compare-and-set then succeeds correctly
	// against the state it actually read. Releasing the writers without this
	// barrier measures scheduling order, not the lost-update the CAS prevents.
	readAll.Add(writers)
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, prior, err := repo.GetTaskDeferredLaunch(ctx, "cas-race")
			readAll.Done()
			<-start
			if err == nil {
				var stored, lost bool
				stored, lost, err = repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-race", prior, newCeilingRecord(int64(i)))
				mu.Lock()
				switch {
				case err != nil:
					failures = append(failures, err)
				case stored:
					stores++
				case lost:
					losses++
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			failures = append(failures, err)
			mu.Unlock()
		}(i)
	}
	readAll.Wait()
	close(start)
	wg.Wait()

	if len(failures) != 0 {
		t.Fatalf("concurrent CAS reported errors: %v", failures)
	}
	if stores != 1 {
		t.Fatalf("stores = %d, want exactly 1 (losses = %d)", stores, losses)
	}
	if losses != writers-1 {
		t.Fatalf("losses = %d, want %d", losses, writers-1)
	}
	if _, ok := readTaskMetadata(t, repo, "cas-race")[models.MetaKeyDeferredLaunch].(map[string]interface{}); !ok {
		t.Fatal("no record survived the race")
	}
}

func jsonNumberOf(t *testing.T, value interface{}) string {
	t.Helper()
	switch typed := value.(type) {
	case json.Number:
		return typed.String()
	case string:
		return typed
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %v: %v", value, err)
		}
		return string(encoded)
	}
}

// TestSetTaskDeferredLaunchLetsALosingWriterReReadAndWin covers the other half of
// the contract: a lost comparison is recoverable, not terminal. The writer that
// lost re-reads, re-applies its rule to what it found, and writes again against
// the fresh prior. This is why a lost compare must not be reported as an error.
func TestSetTaskDeferredLaunchLetsALosingWriterReReadAndWin(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "cas-reread", Title: "Reread"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, stalePrior, err := repo.GetTaskDeferredLaunch(ctx, "cas-reread")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	// The winner lands first.
	if stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-reread", AbsentDeferredLaunch(), newCeilingRecord(100)); err != nil || !stored || lost {
		t.Fatalf("winner = (%v, %v, %v), want (true, false, nil)", stored, lost, err)
	}

	// The loser discovers it lost...
	if stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-reread", stalePrior, newCeilingRecord(200)); err != nil || stored || !lost {
		t.Fatalf("loser = (%v, %v, %v), want (false, true, nil)", stored, lost, err)
	}

	// ...re-reads, re-applies its rule to what is actually stored, and wins.
	record, freshPrior, err := repo.GetTaskDeferredLaunch(ctx, "cas-reread")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	if got := jsonNumberOf(t, record[models.CeilingQueuedAtKey]); got != "100" {
		t.Fatalf("re-read saw ceiling_queued_at %s, want the winner's 100", got)
	}
	record[models.CeilingReasonCodeKey] = "ceiling_superseded"
	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "cas-reread", freshPrior, record)
	if err != nil || !stored || lost {
		t.Fatalf("re-applied write = (%v, %v, %v), want (true, false, nil)", stored, lost, err)
	}

	final, _, err := repo.GetTaskDeferredLaunch(ctx, "cas-reread")
	if err != nil {
		t.Fatalf("GetTaskDeferredLaunch: %v", err)
	}
	// AC-32: the first record's queued_at survives the loser's re-application.
	if got := jsonNumberOf(t, final[models.CeilingQueuedAtKey]); got != "100" {
		t.Fatalf("ceiling_queued_at = %s after re-apply, want the original 100 preserved", got)
	}
	if final[models.CeilingReasonCodeKey] != "ceiling_superseded" {
		t.Fatalf("re-applied reason code missing: %+v", final)
	}
}
