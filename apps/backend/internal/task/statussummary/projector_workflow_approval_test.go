package statussummary

import (
	"encoding/json"
	"testing"
)

func workflowApprovalEvent(state, headSHA, attentionState, attentionHeadSHA string) map[string]interface{} {
	event := map[string]interface{}{
		"repository_id": "repo-1",
		"owner":         "contributor",
		"repo":          "fork",
		"pr_number":     42,
		"state":         state,
		"head_sha":      headSHA,
	}
	if attentionState != "" || attentionHeadSHA != "" {
		event["workflow_attention"] = map[string]interface{}{
			"state":    attentionState,
			"head_sha": attentionHeadSHA,
		}
	}
	return event
}

func assertWorkflowApproval(t *testing.T, got *PullRequestSummary, want bool) {
	t.Helper()
	if got == nil {
		if want {
			t.Fatal("pull request summary is nil, want approval")
		}
		return
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal pull request summary: %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal pull request summary: %v", err)
	}
	actual, _ := fields["workflow_approval_required"].(bool)
	if actual != want {
		t.Fatalf("workflow_approval_required = %v, want %v; payload=%s", actual, want, encoded)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1, AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
func TestWorkflowApprovalProjectionEligibility(t *testing.T) {
	tests := []struct {
		name  string
		event map[string]interface{}
		want  bool
	}{
		{name: "current open head", event: workflowApprovalEvent("open", "head-a", "approval_required", "head-a"), want: true},
		{name: "case-insensitive current head", event: workflowApprovalEvent("open", "HEAD-A", "approval_required", "head-a"), want: true},
		{name: "terminal pull request", event: workflowApprovalEvent("closed", "head-a", "approval_required", "head-a")},
		{name: "head mismatch", event: workflowApprovalEvent("open", "head-b", "approval_required", "head-a")},
		{name: "generic action required", event: workflowApprovalEvent("open", "head-a", "action_required", "head-a")},
		{name: "authoritative none", event: workflowApprovalEvent("open", "head-a", "none", "head-a")},
		{name: "missing observation", event: workflowApprovalEvent("open", "head-a", "", "")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState()
			if !(&Projector{}).applyPREventLocked(state, test.event) {
				t.Fatal("initial PR event was not applied")
			}
			assertWorkflowApproval(t, derivePullRequestSummary(state), test.want)
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.5, AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6
func TestWorkflowApprovalProjectionCarriesDisclosureIdentityAndStaleness(t *testing.T) {
	event := workflowApprovalEvent("open", "head-a", "approval_required", "head-a")
	event["workflow_attention"].(map[string]interface{})["stale"] = true
	event["has_merge_conflicts"] = true
	event["mergeable_state"] = "dirty"
	state := newProjectionState()
	if !(&Projector{}).applyPREventLocked(state, event) {
		t.Fatal("initial PR event was not applied")
	}

	got := derivePullRequestSummary(state)
	if got == nil {
		t.Fatal("pull request summary is nil")
	}
	if !got.WorkflowApprovalRequired || !got.WorkflowApprovalStale {
		t.Fatalf("workflow approval summary = %+v, want stale approval", got)
	}
	if got.WorkflowApprovalPRNumber != 42 || got.WorkflowApprovalRepository != "contributor/fork" {
		t.Fatalf("workflow approval identity = %+v, want contributor/fork PR #42", got)
	}
	if !got.HasMergeConflicts || got.MergeConflictPRNumber != 42 || got.MergeConflictRepository != "contributor/fork" {
		t.Fatalf("merge conflict identity = %+v, want contributor/fork PR #42", got)
	}

	missingRepository := workflowApprovalEvent("open", "head-a", "approval_required", "head-a")
	delete(missingRepository, "owner")
	delete(missingRepository, "repo")
	withoutRepository := newProjectionState()
	if !(&Projector{}).applyPREventLocked(withoutRepository, missingRepository) {
		t.Fatal("PR event without repository identity was not applied")
	}
	if got := derivePullRequestSummary(withoutRepository); got == nil || got.WorkflowApprovalRepository != "" {
		t.Fatalf("incomplete workflow approval repository identity = %+v, want empty", got)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
func TestWorkflowApprovalProjectionClearsOnLifecycle(t *testing.T) {
	tests := []struct {
		name  string
		clear map[string]interface{}
	}{
		{name: "authoritative clearing", clear: workflowApprovalEvent("open", "head-a", "none", "head-a")},
		{name: "head change", clear: workflowApprovalEvent("open", "head-b", "approval_required", "head-a")},
		{name: "closed", clear: workflowApprovalEvent("closed", "head-a", "approval_required", "head-a")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newProjectionState()
			projector := &Projector{}
			if !projector.applyPREventLocked(state, workflowApprovalEvent("open", "head-a", "approval_required", "head-a")) {
				t.Fatal("initial approval event was not applied")
			}
			if !projector.applyPREventLocked(state, test.clear) {
				t.Fatal("clearing PR event was not applied")
			}
			assertWorkflowApproval(t, derivePullRequestSummary(state), false)
		})
	}

	state := newProjectionState()
	state.prObserved = true
	state.prs = map[string]pullRequestObservation{}
	assertWorkflowApproval(t, derivePullRequestSummary(state), false)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6
func TestWorkflowApprovalProjectionAggregatesOpenPRs(t *testing.T) {
	state := newProjectionState()
	projector := &Projector{}
	for _, event := range []map[string]interface{}{
		workflowApprovalEvent("open", "head-a", "none", "head-a"),
		{"repository_id": "repo-2", "pr_number": 43, "state": "open", "head_sha": "head-b", "workflow_attention": map[string]interface{}{"state": "approval_required", "head_sha": "head-b"}},
		{"repository_id": "repo-3", "pr_number": 44, "state": "merged", "head_sha": "head-c", "workflow_attention": map[string]interface{}{"state": "approval_required", "head_sha": "head-c"}},
	} {
		if !projector.applyPREventLocked(state, event) {
			t.Fatalf("PR event was not applied: %#v", event)
		}
	}
	assertWorkflowApproval(t, derivePullRequestSummary(state), true)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
func TestWorkflowApprovalProjectionEventRebuildParity(t *testing.T) {
	var input RebuildInput
	if err := json.Unmarshal([]byte(`{"PRObserved":true,"PullRequests":[{"Key":"repo-1#42","Owner":"contributor","Repo":"fork","State":"open","Number":42,"HeadSHA":"head-a","WorkflowAttentionState":"approval_required","WorkflowAttentionHeadSHA":"head-a","WorkflowAttentionStale":true,"HasMergeConflicts":true,"MergeableState":"dirty"}]}`), &input); err != nil {
		t.Fatalf("unmarshal rebuild fixture: %v", err)
	}
	rebuilt := BuildFromAuthoritative(input).PullRequest

	state := newProjectionState()
	event := workflowApprovalEvent("open", "head-a", "approval_required", "head-a")
	event["workflow_attention"].(map[string]interface{})["stale"] = true
	event["has_merge_conflicts"] = true
	event["mergeable_state"] = "dirty"
	if !(&Projector{}).applyPREventLocked(state, event) {
		t.Fatal("PR event was not applied")
	}
	eventSummary := derivePullRequestSummary(state)
	assertWorkflowApproval(t, rebuilt, true)
	assertWorkflowApproval(t, eventSummary, true)
	if rebuilt.WorkflowApprovalPRNumber != eventSummary.WorkflowApprovalPRNumber ||
		rebuilt.WorkflowApprovalRepository != eventSummary.WorkflowApprovalRepository ||
		rebuilt.WorkflowApprovalStale != eventSummary.WorkflowApprovalStale ||
		rebuilt.MergeConflictPRNumber != eventSummary.MergeConflictPRNumber ||
		rebuilt.MergeConflictRepository != eventSummary.MergeConflictRepository {
		t.Fatalf("rebuild/event workflow approval mismatch: rebuilt=%+v event=%+v", rebuilt, eventSummary)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
func TestWorkflowApprovalProjectionIgnoresMalformedEventEvidence(t *testing.T) {
	state := newProjectionState()
	if !(&Projector{}).applyPREventLocked(state, map[string]interface{}{
		"repository_id": "repo-1",
		"pr_number":     42,
		"state":         "open",
		"head_sha":      "head-a",
		"workflow_attention": map[string]interface{}{
			"state":    "approval_required",
			"head_sha": []string{"head-a"},
		},
	}) {
		t.Fatal("PR event was not applied")
	}
	assertWorkflowApproval(t, derivePullRequestSummary(state), false)
}
