package github

import (
	"context"
	"testing"
	"time"
)

// Creating a watch kicks off an initial poll in the background, and that poll
// stamps LastPolledAt. The watch handed back to the caller is JSON-encoded by
// the HTTP/WS layer, so the goroutine must work on its own copy — otherwise the
// encoder reads LastPolledAt while the poll writes it (a real data race), and
// the caller sees a "last polled" time for a watch it just created.
//
// Both tests below wait for the goroutine to persist its timestamp, then assert
// the returned struct was left alone. That is deterministic: sharing the
// pointer again makes them fail on every run, not just under -race.

func TestCreateIssueWatch_InitialCheckDoesNotMutateTheReturnedWatch(t *testing.T) {
	svc, store := setupWatchServiceTest(t)
	ctx := context.Background()

	watch, err := svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1",
		Repos:       []RepoFilter{{Owner: "acme", Name: "widget"}},
		Prompt:      "fix it",
	})
	if err != nil {
		t.Fatalf("create issue watch: %v", err)
	}

	waitForIssueWatchPolled(t, store, watch.ID)
	if watch.LastPolledAt != nil {
		t.Errorf("returned watch has LastPolledAt = %v, want it untouched by the "+
			"background check", watch.LastPolledAt)
	}
}

func TestCreateReviewWatch_InitialCheckDoesNotMutateTheReturnedWatch(t *testing.T) {
	svc, store := setupWatchServiceTest(t)
	ctx := context.Background()

	watch, err := svc.CreateReviewWatch(ctx, &CreateReviewWatchRequest{
		WorkspaceID: "ws-1",
		Repos:       []RepoFilter{{Owner: "acme", Name: "widget"}},
	})
	if err != nil {
		t.Fatalf("create review watch: %v", err)
	}

	waitForReviewWatchPolled(t, store, watch.ID)
	if watch.LastPolledAt != nil {
		t.Errorf("returned watch has LastPolledAt = %v, want it untouched by the "+
			"background check", watch.LastPolledAt)
	}
}

// waitForIssueWatchPolled blocks until the background check has persisted a
// LastPolledAt for the watch. Real subprocess-free goroutine work, so a short
// polling wait rather than synctest (which cannot advance a real DB write).
func waitForIssueWatchPolled(t *testing.T, store *Store, id string) {
	t.Helper()
	waitForWatchPolled(t, func() (bool, error) {
		stored, err := store.GetIssueWatch(context.Background(), id)
		if err != nil || stored == nil {
			return false, err
		}
		return stored.LastPolledAt != nil, nil
	})
}

func waitForReviewWatchPolled(t *testing.T, store *Store, id string) {
	t.Helper()
	waitForWatchPolled(t, func() (bool, error) {
		stored, err := store.GetReviewWatch(context.Background(), id)
		if err != nil || stored == nil {
			return false, err
		}
		return stored.LastPolledAt != nil, nil
	})
}

func waitForWatchPolled(t *testing.T, polled func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		done, err := polled()
		if err != nil {
			t.Fatalf("read watch: %v", err)
		}
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("initial check never recorded LastPolledAt")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
