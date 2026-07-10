package github

import (
	"context"
	"testing"
	"time"
)

func TestServiceEffectiveCIAutoFixPromptUsesOverrideThenDefault(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(&stubClient{}, "pat", nil, store, nil, testLogger(t))
	svc.SetPromptResolver(staticPromptResolver{content: "resolved default"})
	ctx := context.Background()

	override := "  task-specific prompt  "
	got, usingDefault := svc.effectiveCIAutoFixPrompt(ctx, &TaskCIOptions{
		TaskID:                "task-1",
		AutoFixPromptOverride: &override,
	})
	if got != "task-specific prompt" || usingDefault {
		t.Fatalf("effective prompt=%q usingDefault=%v, want override and false", got, usingDefault)
	}

	emptyOverride := "  "
	got, usingDefault = svc.effectiveCIAutoFixPrompt(ctx, &TaskCIOptions{
		TaskID:                "task-1",
		AutoFixPromptOverride: &emptyOverride,
	})
	if got != "resolved default" || !usingDefault {
		t.Fatalf("effective prompt=%q usingDefault=%v, want resolved default and true", got, usingDefault)
	}
}

func TestServiceTaskCIPRStatesMergesStoredAndCurrentPRs(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(&stubClient{}, "pat", nil, store, nil, testLogger(t))
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-front",
		Owner:        "acme",
		Repo:         "front",
		PRNumber:     1,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create front PR: %v", err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-back",
		Owner:        "acme",
		Repo:         "back",
		PRNumber:     2,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create back PR: %v", err)
	}
	if err := store.RecordTaskCIError(ctx, "task-1", "repo-front", 1, "fix failed"); err != nil {
		t.Fatalf("record current PR state: %v", err)
	}
	if err := store.RecordTaskCIError(ctx, "task-1", "repo-old", 9, "old state"); err != nil {
		t.Fatalf("record orphan PR state: %v", err)
	}

	states, err := svc.taskCIPRStates(ctx, "task-1")
	if err != nil {
		t.Fatalf("taskCIPRStates: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("len(states)=%d, want 3: %+v", len(states), states)
	}
	byKey := make(map[string]*TaskCIPRAutomationState, len(states))
	for _, state := range states {
		byKey[taskCIPRStateKey(state.RepositoryID, state.PRNumber)] = state
	}
	if got := byKey[taskCIPRStateKey("repo-front", 1)]; got == nil || got.LastError == nil || *got.LastError != "fix failed" {
		t.Fatalf("front state=%+v, want stored error", got)
	}
	if got := byKey[taskCIPRStateKey("repo-back", 2)]; got == nil || got.LastError != nil || got.LastFixSignature != "" {
		t.Fatalf("back state=%+v, want placeholder without stored automation state", got)
	}
	if got := byKey[taskCIPRStateKey("repo-old", 9)]; got == nil || got.LastError == nil || *got.LastError != "old state" {
		t.Fatalf("orphan state=%+v, want retained stored state", got)
	}
}
