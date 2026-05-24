package main

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeSessionMsgRepo is a minimal stub for taskSessionCheckerAdapter.repo
// — returns canned sessions + per-session messages from in-memory maps.
type fakeSessionMsgRepo struct {
	sessions map[string][]*models.TaskSession
	messages map[string][]*models.Message
}

func (r *fakeSessionMsgRepo) ListTaskSessions(_ context.Context, taskID string) ([]*models.TaskSession, error) {
	return r.sessions[taskID], nil
}

func (r *fakeSessionMsgRepo) ListMessages(_ context.Context, sessionID string) ([]*models.Message, error) {
	return r.messages[sessionID], nil
}

func TestHasUserAuthoredMessage(t *testing.T) {
	cases := []struct {
		name     string
		messages []*models.Message
		want     bool
	}{
		{
			name: "agent-only messages → false",
			messages: []*models.Message{
				{AuthorType: models.MessageAuthorAgent, Content: "hi"},
			},
			want: false,
		},
		{
			name: "user message tagged auto_start=true → false (PR/issue/Jira/Linear watch path)",
			messages: []*models.Message{
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "Review this PR...",
					Metadata:   map[string]interface{}{"auto_start": true},
				},
			},
			want: false,
		},
		{
			// Regression for claude blocker on b89f880c: recordAutoStartMessage
			// (workflow auto-start path) used to tag only workflow_auto_start
			// without also setting auto_start, so messages slipped past
			// HasUserAuthoredMessage and made the task look user-authored.
			// The fix sets both flags; assert this filter recognizes either.
			name: "workflow auto-start (auto_start=true, workflow_auto_start=true) → false",
			messages: []*models.Message{
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "{{task_prompt}}\nDo the thing",
					Metadata: map[string]interface{}{
						"auto_start":          true,
						"workflow_auto_start": true,
						"plan_mode":           true,
					},
				},
			},
			want: false,
		},
		{
			// Legacy message from before the cleanup_policy work:
			// recordAutoStartMessage only tagged workflow_auto_start. The
			// filter must recognize that tag too so installs with piled-up
			// pre-upgrade tasks can drain them via the manual button.
			name: "legacy workflow auto-start (only workflow_auto_start=true) → false",
			messages: []*models.Message{
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "legacy auto-start prompt",
					Metadata:   map[string]interface{}{"workflow_auto_start": true},
				},
			},
			want: false,
		},
		{
			name: "real user message (no auto_start tag) → true",
			messages: []*models.Message{
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "looks good, ship it",
				},
			},
			want: true,
		},
		{
			name: "auto-start message + later real user message → true",
			messages: []*models.Message{
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "Review this PR...",
					Metadata:   map[string]interface{}{"auto_start": true},
				},
				{
					AuthorType: models.MessageAuthorAgent,
					Content:    "Looking at the diff",
				},
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "thanks",
				},
			},
			want: true,
		},
		{
			name: "auto_start metadata value of false → still counted as user",
			messages: []*models.Message{
				{
					AuthorType: models.MessageAuthorUser,
					Content:    "real input",
					Metadata:   map[string]interface{}{"auto_start": false},
				},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &taskSessionCheckerAdapter{
				repo: &fakeSessionMsgRepo{
					sessions: map[string][]*models.TaskSession{
						"task-1": {{ID: "sess-1"}},
					},
					messages: map[string][]*models.Message{
						"sess-1": tc.messages,
					},
				},
			}
			got, err := adapter.HasUserAuthoredMessage(context.Background(), "task-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("HasUserAuthoredMessage = %v, want %v", got, tc.want)
			}
		})
	}
}
