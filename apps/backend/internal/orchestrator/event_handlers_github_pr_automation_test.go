package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/github"
)

func TestDecideTaskPRAgentPrompt(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name            string
		prState         string
		options         *github.TaskCIOptionsResponse
		checkpoint      *github.TaskCIPRAutomationState
		reviewRequested *bool
		wantEvent       string
		wantReviewStamp *bool
		wantStateStamp  string
	}{
		{
			name:            "initial review request establishes baseline",
			prState:         "open",
			options:         &github.TaskCIOptionsResponse{PromptOnReviewRequested: true},
			checkpoint:      &github.TaskCIPRAutomationState{},
			reviewRequested: &trueValue,
			wantReviewStamp: &trueValue,
			wantStateStamp:  "open",
		},
		{
			name:            "cleared review request rearms without prompting",
			prState:         "open",
			options:         &github.TaskCIOptionsResponse{PromptOnReviewRequested: true},
			checkpoint:      &github.TaskCIPRAutomationState{ReviewRequestInitialized: true, LastReviewRequested: true, LastObservedPRState: "open"},
			reviewRequested: &falseValue,
			wantReviewStamp: &falseValue,
		},
		{
			name:            "new review request prompts after rearm",
			prState:         "open",
			options:         &github.TaskCIOptionsResponse{PromptOnReviewRequested: true},
			checkpoint:      &github.TaskCIPRAutomationState{ReviewRequestInitialized: true, LastReviewRequested: false, LastObservedPRState: "open"},
			reviewRequested: &trueValue,
			wantEvent:       taskPRAgentEventReviewRequested,
			wantReviewStamp: &trueValue,
		},
		{
			name:           "merged PR prompts once",
			prState:        "merged",
			options:        &github.TaskCIOptionsResponse{PromptOnMerged: true},
			checkpoint:     &github.TaskCIPRAutomationState{LastObservedPRState: "open"},
			wantEvent:      taskPRAgentEventMerged,
			wantStateStamp: "merged",
		},
		{
			name:       "stable merged PR stays quiet",
			prState:    "merged",
			options:    &github.TaskCIOptionsResponse{PromptOnMerged: true},
			checkpoint: &github.TaskCIPRAutomationState{LastObservedPRState: "merged"},
		},
		{
			name:       "enabling another terminal option does not repeat delivered merge",
			prState:    "merged",
			options:    &github.TaskCIOptionsResponse{PromptOnMerged: true, PromptOnClosed: true},
			checkpoint: &github.TaskCIPRAutomationState{LastLifecycleEvent: "merged"},
		},
		{
			name:           "closed PR prompts when subscribed",
			prState:        "closed",
			options:        &github.TaskCIOptionsResponse{PromptOnClosed: true},
			checkpoint:     nil,
			wantEvent:      taskPRAgentEventClosed,
			wantStateStamp: "closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideTaskPRAgentPrompt(tt.prState, tt.options, tt.checkpoint, tt.reviewRequested)
			if got.Event != tt.wantEvent {
				t.Fatalf("Event = %q, want %q", got.Event, tt.wantEvent)
			}
			if got.ObservedState != tt.wantStateStamp {
				t.Fatalf("ObservedState = %q, want %q", got.ObservedState, tt.wantStateStamp)
			}
			if !equalOptionalBool(got.ReviewRequested, tt.wantReviewStamp) {
				t.Fatalf("ReviewRequested = %v, want %v", got.ReviewRequested, tt.wantReviewStamp)
			}
		})
	}
}

func equalOptionalBool(left, right *bool) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
