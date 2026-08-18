package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/github"
)

func TestResolvePRScopedCIOptions(t *testing.T) {
	override := "task prompt"
	options := &github.TaskCIOptionsResponse{
		TaskID:                  "task-1",
		AutoFixEnabled:          true,
		AutoMergeEnabled:        true,
		PromptOnReviewRequested: true,
		PromptOnMerged:          true,
		PromptOnClosed:          true,
		ReviewReviewerLogin:     "reviewer",
		AutoFixPromptOverride:   &override,
		EffectiveAutoFixPrompt:  "effective prompt",
		PROptions: []*github.TaskPRAutomationOptions{
			{
				RepositoryID: "repo-1", PRNumber: 1,
				AutoFixEnabled: false, AutoMergeEnabled: false,
				PromptOnReviewRequested: false, PromptOnMerged: false, PromptOnClosed: false,
			},
			{
				RepositoryID: "repo-2", PRNumber: 2,
				AutoFixEnabled: true, AutoMergeEnabled: false,
				PromptOnReviewRequested: true, PromptOnMerged: false, PromptOnClosed: true,
			},
		},
	}
	pr := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 1}
	got := resolvePRScopedCIOptions(options, pr)
	if got == options {
		t.Fatal("resolver returned the input response instead of a scoped copy")
	}
	if got.AutoFixEnabled || got.AutoMergeEnabled || got.PromptOnReviewRequested || got.PromptOnMerged || got.PromptOnClosed {
		t.Fatalf("scoped switches = %+v, want PR #1 values", got)
	}
	if got.ReviewReviewerLogin != options.ReviewReviewerLogin ||
		got.AutoFixPromptOverride != options.AutoFixPromptOverride ||
		got.EffectiveAutoFixPrompt != options.EffectiveAutoFixPrompt {
		t.Fatalf("task-level fields changed during scoping: got=%+v want=%+v", got, options)
	}

	missing := resolvePRScopedCIOptions(options, &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-3", PRNumber: 3})
	if !missing.AutoFixEnabled || !missing.AutoMergeEnabled || !missing.PromptOnReviewRequested || !missing.PromptOnMerged || !missing.PromptOnClosed {
		t.Fatal("missing PR row did not preserve the task aggregate fallback")
	}

	withNil := *options
	withNil.PROptions = append([]*github.TaskPRAutomationOptions{nil}, options.PROptions...)
	if scoped := resolvePRScopedCIOptions(&withNil, &github.TaskPR{RepositoryID: "repo-2", PRNumber: 2}); !scoped.AutoFixEnabled {
		t.Fatal("nil PR option entry prevented matching row resolution")
	}
	if resolvePRScopedCIOptions(nil, pr) != nil || resolvePRScopedCIOptions(options, nil) != options {
		t.Fatal("nil input handling changed")
	}
}

func TestAnyTaskPRAutomationEnabled(t *testing.T) {
	for _, test := range []struct {
		name    string
		options *github.TaskCIOptionsResponse
		want    bool
	}{
		{
			name: "mixed per PR options",
			options: &github.TaskCIOptionsResponse{PROptions: []*github.TaskPRAutomationOptions{
				{RepositoryID: "repo-1", PRNumber: 1},
				{RepositoryID: "repo-2", PRNumber: 2, AutoMergeEnabled: true},
			}},
			want: true,
		},
		{
			name: "all off",
			options: &github.TaskCIOptionsResponse{PROptions: []*github.TaskPRAutomationOptions{
				{RepositoryID: "repo-1", PRNumber: 1},
				{RepositoryID: "repo-2", PRNumber: 2},
			}},
			want: false,
		},
		{
			name:    "empty per PR options falls back to aggregate",
			options: &github.TaskCIOptionsResponse{AutoFixEnabled: true},
			want:    true,
		},
		{
			name:    "nil entry is skipped",
			options: &github.TaskCIOptionsResponse{PROptions: []*github.TaskPRAutomationOptions{nil}},
			want:    false,
		},
		{
			name:    "nil response",
			options: nil,
			want:    false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := anyTaskPRAutomationEnabled(test.options); got != test.want {
				t.Fatalf("anyTaskPRAutomationEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}
