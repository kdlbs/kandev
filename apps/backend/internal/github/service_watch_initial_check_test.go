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

func TestCreateIssueWatch_InitialCheckDoesNotOverwriteConcurrentUpdate(t *testing.T) {
	svc, store := setupWatchServiceTest(t)
	client := &blockingIssueWatchCheckClient{
		stubClient: &stubClient{},
		entered:    make(chan struct{}, 1),
		release:    make(chan struct{}),
	}
	svc.resolver.SetLegacyFactory(func(context.Context) (Client, string, error) {
		return client, AuthMethodPAT, nil
	})

	watch, err := svc.CreateIssueWatch(context.Background(), &CreateIssueWatchRequest{
		WorkspaceID: "ws-1",
		Repos:       []RepoFilter{{Owner: "acme", Name: "widget"}},
		Prompt:      "original prompt",
	})
	if err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	defer func() {
		select {
		case <-client.release:
		default:
			close(client.release)
		}
	}()

	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("initial issue check did not reach the provider")
	}

	updatedPrompt := "updated prompt"
	disabled := false
	if err := svc.UpdateIssueWatch(context.Background(), watch.ID, &UpdateIssueWatchRequest{
		Prompt:  &updatedPrompt,
		Enabled: &disabled,
	}); err != nil {
		t.Fatalf("update issue watch while initial check is blocked: %v", err)
	}
	close(client.release)
	waitForIssueWatchPolled(t, store, watch.ID)

	got, err := store.GetIssueWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("get issue watch after initial check: %v", err)
	}
	if got.Prompt != updatedPrompt || got.Enabled {
		t.Errorf("initial check overwrote concurrent update: prompt = %q, enabled = %v", got.Prompt, got.Enabled)
	}
}

type blockingIssueWatchCheckClient struct {
	*stubClient
	entered chan struct{}
	release chan struct{}
}

func (c *blockingIssueWatchCheckClient) ListIssues(ctx context.Context, _, _ string) ([]*Issue, error) {
	c.entered <- struct{}{}
	select {
	case <-c.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
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
