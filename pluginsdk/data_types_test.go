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
			{ID: "tr-1", RepositoryID: "repo-1", BaseBranch: "main", Position: 0},
			{ID: "tr-2", RepositoryID: "repo-2", BaseBranch: "develop", Position: 1},
		},
		Metadata: map[string]any{"source": "plugin:agent-stats", "count": float64(3)},
	}

	proto, err := task.toProto()
	require.NoError(t, err)
	require.Equal(t, "task-1", proto.GetId())
	require.Equal(t, "2026-07-15T12:01:00Z", proto.GetStartedAt())
	require.Nil(t, proto.CompletedAt)

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

func TestTaskFilterProtoRoundTrip(t *testing.T) {
	filter := TaskFilter{
		WorkspaceIDs:     []string{"ws-1", "ws-2"},
		WorkflowIDs:      []string{"wf-1"},
		States:           []string{"todo", "in_progress"},
		ParentID:         strPtr("task-0"),
		IncludeEphemeral: true,
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
		ID:         "step-1",
		WorkflowID: "wf-1",
		Name:       "Review",
		Position:   1,
		StageType:  "review",
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

func TestRepositoryProtoRoundTrip(t *testing.T) {
	repo := Repository{
		ID:            "repo-1",
		WorkspaceID:   "ws-1",
		Name:          "kdlbs/kandev",
		DefaultBranch: strPtr("main"),
	}
	proto := repo.toProto()
	require.Equal(t, repo, repositoryFromProto(proto))

	repoNoBranch := Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "kdlbs/other"}
	protoNoBranch := repoNoBranch.toProto()
	require.Nil(t, protoNoBranch.DefaultBranch)
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
	}
	proto := stats.toProto()
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
