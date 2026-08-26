package pluginsdk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestPageProtoRoundTrip(t *testing.T) {
	p := Page{Limit: 25, Cursor: "cursor-1"}
	proto := p.toProto()
	require.Equal(t, int32(25), proto.GetLimit())
	require.Equal(t, "cursor-1", proto.GetCursor())
	require.Equal(t, p, pageFromProto(proto))
}

func TestPageInfoProtoRoundTrip(t *testing.T) {
	pi := &PageInfo{NextCursor: "next-1", HasMore: true}
	proto := pi.toProto()
	require.Equal(t, "next-1", proto.GetNextCursor())
	require.True(t, proto.GetHasMore())
	require.Equal(t, pi, pageInfoFromProto(proto))

	// nil PageInfo converts to a nil proto and back to nil.
	var nilPI *PageInfo
	require.Nil(t, nilPI.toProto())
	require.Nil(t, pageInfoFromProto(nil))
}

func TestTaskProtoRoundTrip(t *testing.T) {
	task := Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Fix the bug",
		Description: "Details here",
		State:       "in_progress",
		Priority:    "high",
		CreatedBy:   "user-1",
		CreatedAt:   "2026-07-15T12:00:00Z",
		UpdatedAt:   "2026-07-15T12:05:00Z",
		StartedAt:   strPtr("2026-07-15T12:01:00Z"),
		CompletedAt: nil,
		ParentID:    strPtr("task-0"),
		Identifier:  "PROJ-42",
		IsEphemeral: false,
		Repositories: []TaskRepository{
			{ID: "tr-1", RepositoryID: "repo-1", BaseBranch: "main", Position: 0, CheckoutBranch: "feature/fix"},
			{ID: "tr-2", RepositoryID: "repo-2", BaseBranch: "develop", Position: 1},
		},
		Metadata:   map[string]any{"source": "plugin:agent-stats", "count": float64(3)},
		ArchivedAt: strPtr("2026-07-16T12:00:00Z"),
		PullRequests: []TaskPullRequest{{
			Number: 42, URL: "https://example.test/pulls/42", Title: "Fix the bug",
			State: "open", HeadBranch: "feature/fix", BaseBranch: "main", IsDraft: true,
			Provider: "github", MergedAt: strPtr("2026-07-16T11:00:00Z"),
			ClosedAt: strPtr("2026-07-16T11:30:00Z"), ReviewState: "approved",
			ChecksState: "success", MergeableState: "clean", UnresolvedReviewThreads: 1,
			ChecksTotal: 5, ChecksPassing: 5, Additions: 12, Deletions: 3, AuthorLogin: "nova28",
		}},
		WorkflowStepID: "step-review", Position: 4, AssigneeAgentProfileID: "agent-1",
		Labels: []string{"plugin", "ready"}, Autopilot: true, WIPAdmitted: true,
		QueuedForStepID: "step-build", QueuedAt: strPtr("2026-07-16T10:00:00Z"),
		ProjectID: "project-1", ExternalID: "external-1",
	}

	proto, err := task.toProto()
	require.NoError(t, err)
	require.Equal(t, "task-1", proto.GetId())
	require.Equal(t, "2026-07-15T12:01:00Z", proto.GetStartedAt())
	require.Nil(t, proto.CompletedAt)
	require.Equal(t, "feature/fix", proto.GetRepositories()[0].GetCheckoutBranch())
	require.Empty(t, proto.GetRepositories()[1].GetCheckoutBranch(), "empty checkout branches remain wire-compatible")
	require.Equal(t, "step-review", proto.GetWorkflowStepId())
	require.True(t, proto.GetAutopilot())
	require.Len(t, proto.GetPullRequests(), 1)
	require.Equal(t, int64(42), proto.GetPullRequests()[0].GetNumber())

	back, err := taskFromProto(proto)
	require.NoError(t, err)
	require.Equal(t, task, back)
}

func TestTaskProtoRoundTrip_NilOptionalsAndEmptyMetadata(t *testing.T) {
	task := Task{ID: "task-2", Title: "Bare task"}

	proto, err := task.toProto()
	require.NoError(t, err)
	require.Nil(t, proto.StartedAt)
	require.Nil(t, proto.CompletedAt)
	require.Nil(t, proto.ParentId)
	require.Nil(t, proto.GetMetadata())
	require.Nil(t, proto.GetRepositories())

	back, err := taskFromProto(proto)
	require.NoError(t, err)
	require.Equal(t, task, back)
}

func TestTaskPullRequestProtoRoundTrip(t *testing.T) {
	pr := TaskPullRequest{
		Number: 42, URL: "https://example.test/pulls/42", Title: "Fix the bug",
		State: "merged", HeadBranch: "feature/fix", BaseBranch: "main", IsDraft: false,
		Provider: "github", MergedAt: strPtr("2026-07-16T11:00:00Z"), ClosedAt: nil,
		ReviewState: "approved", ChecksState: "success", MergeableState: "clean",
		UnresolvedReviewThreads: 0, ChecksTotal: 5, ChecksPassing: 5,
		Additions: 12, Deletions: 3, AuthorLogin: "nova28",
	}

	proto := pr.toProto()
	require.Equal(t, pr, taskPullRequestFromProto(proto))
	require.Nil(t, proto.ClosedAt)
}

func TestTaskFilterProtoRoundTrip(t *testing.T) {
	filter := TaskFilter{
		WorkspaceIDs:     []string{"ws-1", "ws-2"},
		WorkflowIDs:      []string{"wf-1"},
		States:           []string{"todo", "in_progress"},
		ParentID:         strPtr("task-0"),
		IncludeEphemeral: true,
		IncludeArchived:  true,
	}
	proto := filter.toProto()
	require.Equal(t, filter, taskFilterFromProto(proto))

	// nil filter proto converts to the zero value.
	require.Equal(t, TaskFilter{}, taskFilterFromProto(nil))
}

func TestWorkspaceProtoRoundTrip(t *testing.T) {
	ws := Workspace{
		ID:                    "ws-1",
		Name:                  "Acme",
		Description:           strPtr("Acme workspace"),
		OwnerID:               "user-1",
		DefaultExecutorID:     strPtr("exec-1"),
		DefaultAgentProfileID: nil,
		CreatedAt:             "2026-07-15T12:00:00Z",
		UpdatedAt:             "2026-07-15T12:05:00Z",
	}
	proto := ws.toProto()
	require.Nil(t, proto.DefaultAgentProfileId)
	require.Equal(t, ws, workspaceFromProto(proto))
}

func TestWorkflowProtoRoundTrip(t *testing.T) {
	wf := Workflow{
		ID:          "wf-1",
		WorkspaceID: "ws-1",
		Name:        "Default",
		Description: nil,
		SortOrder:   2,
		CreatedAt:   "2026-07-15T12:00:00Z",
		UpdatedAt:   "2026-07-15T12:05:00Z",
	}
	proto := wf.toProto()
	require.Equal(t, wf, workflowFromProto(proto))
}

func TestWorkflowStepProtoRoundTrip(t *testing.T) {
	step := WorkflowStep{
		ID:             "step-1",
		WorkflowID:     "wf-1",
		Name:           "Review",
		Position:       1,
		StageType:      "review",
		Color:          "bg-indigo-500",
		IsStartStep:    true,
		WIPLimit:       3,
		AgentProfileID: "agent-1",
	}
	proto := step.toProto()
	require.Equal(t, step, workflowStepFromProto(proto))
}

func TestWorkflowStepProtoRoundTrip_OnEnterActionTypes(t *testing.T) {
	step := WorkflowStep{
		ID:                 "step-1",
		WorkflowID:         "wf-1",
		Name:               "Work",
		Position:           1,
		StageType:          "work",
		OnEnterActionTypes: []string{"auto_start_agent", "run_code_review", "set_session_mode"},
	}
	proto := step.toProto()
	require.Equal(t, step, workflowStepFromProto(proto))
}

func TestAgentProfileProtoRoundTrip(t *testing.T) {
	profile := AgentProfile{
		ID:          "profile-1",
		AgentID:     "claude",
		DisplayName: "Claude Sonnet",
		Name:        "claude-sonnet",
		Model:       "claude-sonnet-5",
		Mode:        "code",
	}
	proto := profile.toProto()
	require.Equal(t, profile, agentProfileFromProto(proto))
}

func TestExecutorProfileProtoRoundTrip(t *testing.T) {
	profile := ExecutorProfile{ID: "exec-1", DisplayName: "Local Docker", ExecutorType: "local_docker"}
	require.Equal(t, profile, executorProfileFromProto(profile.toProto()))
}

func TestCreateTaskInputRichProtoRoundTrip(t *testing.T) {
	defaultBranch := "main"
	baseBranch := "release"
	headBranch := "feature/plugin"
	checkoutBranch := "feature/plugin"
	pullRequestNumber := int64(42)
	prompt := "Implement the fix"
	input := CreateTaskInput{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Plugin-created task",
		Repositories: []PluginTaskRepository{{
			Remote: &RemoteRepositoryDescriptor{
				ProviderID: "example", ProviderHost: "code.example.test", OwnerOrProject: "team",
				ProviderRepositoryID: "repo-42", Name: "api", CloneURL: "https://code.example.test/scm/team/api.git",
				DefaultBranch: &defaultBranch, BaseBranch: &baseBranch, HeadBranch: &headBranch, PullRequestNumber: &pullRequestNumber,
			},
			BaseBranch: &baseBranch, CheckoutBranch: &checkoutBranch, PullRequestNumber: &pullRequestNumber,
		}},
		Launch:   &PluginTaskLaunchOptions{AgentProfileID: strPtr("agent-1"), ExecutorProfileID: strPtr("exec-1"), Prompt: &prompt, PlanMode: strPtr("replace")},
		Metadata: map[string]any{"watch_id": "watch-1"},
	}

	proto, err := input.toProto()
	require.NoError(t, err)
	back, err := createTaskInputFromProto(proto)
	require.NoError(t, err)
	require.Equal(t, input, back)
}

func TestRepositoryProtoRoundTrip(t *testing.T) {
	repo := Repository{
		ID:                   "repo-1",
		WorkspaceID:          "ws-1",
		Name:                 "kdlbs/kandev",
		DefaultBranch:        strPtr("main"),
		SourceType:           "provider",
		ProviderID:           "example-vcs",
		ProviderRepositoryID: "repo-42",
		ProviderHost:         "code.example.test",
		OwnerOrProject:       "team",
		ProviderName:         "kandev",
		RemoteURL:            "https://code.example.test/scm/team/kandev.git",
	}
	proto := repo.toProto()
	require.Equal(t, repo, repositoryFromProto(proto))

	repoNoBranch := Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "kdlbs/other"}
	protoNoBranch := repoNoBranch.toProto()
	require.Nil(t, protoNoBranch.DefaultBranch)
	require.Empty(t, protoNoBranch.GetProviderId(), "new origin fields preserve empty compatibility")
	require.Empty(t, protoNoBranch.GetRemoteUrl(), "new origin fields preserve empty compatibility")
	require.Equal(t, repoNoBranch, repositoryFromProto(protoNoBranch))
}

func TestSessionProtoRoundTrip(t *testing.T) {
	session := Session{
		ID:               "session-1",
		TaskID:           "task-1",
		AgentProfileID:   "profile-1",
		AgentDisplayName: "Claude Sonnet",
		Model:            "claude-sonnet-5",
		ACPSessionID:     "acp-1",
		State:            "running",
		StartedAt:        "2026-07-15T12:00:00Z",
		EndedAt:          nil,
	}
	proto := session.toProto()
	require.Nil(t, proto.EndedAt)
	require.Equal(t, session, sessionFromProto(proto))

	ended := session
	ended.EndedAt = strPtr("2026-07-15T13:00:00Z")
	protoEnded := ended.toProto()
	require.Equal(t, ended, sessionFromProto(protoEnded))
}

func TestSessionFilterProtoRoundTrip(t *testing.T) {
	filter := SessionFilter{
		TaskIDs:      []string{"task-1", "task-2"},
		WorkspaceIDs: []string{"ws-1"},
		States:       []string{"running"},
	}
	proto := filter.toProto()
	require.Equal(t, filter, sessionFilterFromProto(proto))
	require.Equal(t, SessionFilter{}, sessionFilterFromProto(nil))
}

func TestSessionCodeStatsProtoRoundTrip(t *testing.T) {
	stats := SessionCodeStats{
		SessionID:               "session-1",
		LinesAddedCommitted:     120,
		LinesDeletedCommitted:   40,
		LinesAddedPeakPending:   15,
		LinesDeletedPeakPending: 3,
		CommittedLinesAvailable: true,
	}
	proto := stats.toProto()
	require.Equal(t, stats, sessionCodeStatsFromProto(proto))
}

// A session that predates commit-capture activation reports
// CommittedLinesAvailable == false, not a real measurement, for committed
// lines — the wire contract must round-trip that unavailability exactly,
// not silently present it as a real zero-change session. LinesAddedCommitted/
// LinesDeletedCommitted stay plain int64 (not pointers) because they are
// already-shipped public SDK fields (ADR 0043's additive-only DTO contract);
// see SessionCodeStats' doc comment.
func TestSessionCodeStatsProtoRoundTrip_CommittedLinesUnavailable(t *testing.T) {
	stats := SessionCodeStats{
		SessionID:               "session-legacy",
		LinesAddedCommitted:     0,
		LinesDeletedCommitted:   0,
		LinesAddedPeakPending:   7,
		LinesDeletedPeakPending: 1,
		CommittedLinesAvailable: false,
	}
	proto := stats.toProto()
	if proto.GetCommittedLinesAvailable() {
		t.Errorf("proto.CommittedLinesAvailable = true, want false")
	}
	require.Equal(t, stats, sessionCodeStatsFromProto(proto))
}

func TestTasksSliceProtoRoundTrip_EmptyIsNil(t *testing.T) {
	tasks, err := tasksFromProto(nil)
	require.NoError(t, err)
	require.Nil(t, tasks)

	protoTasks, err := tasksToProto(nil)
	require.NoError(t, err)
	require.Nil(t, protoTasks)
}
