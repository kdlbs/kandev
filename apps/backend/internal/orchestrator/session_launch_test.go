package orchestrator

import (
	"testing"
)

func TestResolveIntent(t *testing.T) {
	tests := []struct {
		name string
		req  LaunchSessionRequest
		want SessionIntent
	}{
		// Explicit intents take priority
		{
			name: "explicit start intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentStart},
			want: IntentStart,
		},
		{
			name: "explicit resume intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentResume, SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "explicit prepare intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentPrepare},
			want: IntentPrepare,
		},
		{
			name: "explicit start_created intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentStartCreated, SessionID: "s1"},
			want: IntentStartCreated,
		},
		{
			name: "explicit workflow_step intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentWorkflowStep, SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},

		// Inferred intents (no explicit intent set)
		{
			name: "workflow_step inferred from session_id + workflow_step_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},
		{
			name: "resume inferred from session_id only",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "resume inferred from session_id with no prompt and no agent_profile",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "start_created inferred from session_id + prompt",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", Prompt: "hello"},
			want: IntentStartCreated,
		},
		{
			name: "start_created inferred from session_id + agent_profile_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", AgentProfileID: "ap1"},
			want: IntentStartCreated,
		},
		{
			name: "start_created inferred from session_id + prompt + agent_profile_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", Prompt: "hello", AgentProfileID: "ap1"},
			want: IntentStartCreated,
		},
		{
			name: "prepare inferred from launch_workspace without prompt",
			req:  LaunchSessionRequest{TaskID: "t1", LaunchWorkspace: true},
			want: IntentPrepare,
		},
		{
			name: "start inferred from minimal request",
			req:  LaunchSessionRequest{TaskID: "t1"},
			want: IntentStart,
		},
		{
			name: "start inferred when prompt provided without session_id",
			req:  LaunchSessionRequest{TaskID: "t1", Prompt: "do something"},
			want: IntentStart,
		},
		{
			name: "start inferred when launch_workspace + prompt (not prepare)",
			req:  LaunchSessionRequest{TaskID: "t1", LaunchWorkspace: true, Prompt: "do something"},
			want: IntentStart,
		},

		// Edge cases
		{
			name: "resume wins over start_created when session_id set, no prompt, no agent_profile",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", ExecutorID: "e1"},
			want: IntentResume,
		},
		{
			name: "workflow_step wins over resume when both session_id and workflow_step_id set",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveIntent(&tt.req)
			if got != tt.want {
				t.Errorf("ResolveIntent() = %q, want %q", got, tt.want)
			}
		})
	}
}
