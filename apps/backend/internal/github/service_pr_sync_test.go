package github

import (
	"context"
	"testing"
)

func TestTriggerPRSync_SyncsExistingWatch(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a PR in mock client.
	mockClient.AddPR(&PR{
		Number:     10,
		Title:      "Feature PR",
		State:      "open",
		HeadSHA:    "abc123",
		HeadBranch: "feat/x",
		RepoOwner:  "org",
		RepoName:   "repo",
	})

	// Create a PR watch and TaskPR in DB.
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "org",
		Repo:      "repo",
		PRNumber:  10,
		Branch:    "feat/x",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	tp := &TaskPR{
		TaskID:   "t1",
		Owner:    "org",
		Repo:     "repo",
		PRNumber: 10,
		PRTitle:  "Feature PR",
		State:    "open",
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatal(err)
	}

	// Trigger sync.
	result, err := svc.TriggerPRSync(ctx, "t1")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil TaskPR")
	}
	if result.LastSyncedAt == nil {
		t.Error("expected LastSyncedAt to be set after sync")
	}
}

func TestTriggerPRSync_DetectsPR(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a PR findable by branch.
	mockClient.AddPR(&PR{
		Number:     20,
		Title:      "New PR",
		State:      "open",
		HeadBranch: "feat/y",
		RepoOwner:  "org",
		RepoName:   "repo",
	})

	// Create a watch with pr_number=0 (still searching).
	watch := &PRWatch{
		SessionID: "s2",
		TaskID:    "t2",
		Owner:     "org",
		Repo:      "repo",
		PRNumber:  0,
		Branch:    "feat/y",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}

	result, err := svc.TriggerPRSync(ctx, "t2")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result == nil {
		t.Fatal("expected TaskPR after detection")
	}
	if result.PRNumber != 20 {
		t.Errorf("expected PR #20, got #%d", result.PRNumber)
	}
}

func TestTriggerPRSync_NoWatch(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()

	result, err := svc.TriggerPRSync(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil TaskPR for task without watch, got %+v", result)
	}
}
