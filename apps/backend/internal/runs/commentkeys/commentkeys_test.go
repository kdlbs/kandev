package commentkeys

import "testing"

func TestIdentityFromPayload(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantTask    string
		wantComment string
	}{
		{
			name:        "task and comment",
			payload:     `{"task_id":"target-task","comment_id":"cm-1"}`,
			wantTask:    "target-task",
			wantComment: "cm-1",
		},
		{
			name:        "source task overrides task",
			payload:     `{"task_id":"target-task","source_task_id":"source-task","comment_id":"cm-1"}`,
			wantTask:    "source-task",
			wantComment: "cm-1",
		},
		{
			name:        "comment id only",
			payload:     `{"comment_id":"cm-1"}`,
			wantComment: "cm-1",
		},
		{
			name:     "source task without comment",
			payload:  `{"task_id":"target-task","source_task_id":"source-task"}`,
			wantTask: "source-task",
		},
		{
			name:    "malformed",
			payload: `{`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTask, gotComment := IdentityFromPayload(tt.payload)
			if gotTask != tt.wantTask || gotComment != tt.wantComment {
				t.Fatalf("IdentityFromPayload() = (%q, %q), want (%q, %q)",
					gotTask, gotComment, tt.wantTask, tt.wantComment)
			}
		})
	}
}

func TestCommentIDFromKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "task_comment:cm-1", want: "cm-1"},
		{key: "task_comment:cm-1:step-1:task-1:agent-1:abcd", want: "cm-1"},
		{key: "other:cm-1", want: ""},
	}
	for _, tt := range tests {
		if got := CommentIDFromKey(tt.key); got != tt.want {
			t.Fatalf("CommentIDFromKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
