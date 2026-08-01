package manifest

import "testing"

func TestMatchSubject(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		subject string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "task.created",
			subject: "task.created",
			want:    true,
		},
		{
			name:    "trailing wildcard matches one segment",
			pattern: "task.*",
			subject: "task.created",
			want:    true,
		},
		{
			name:    "trailing wildcard matches a different single segment",
			pattern: "office.*",
			subject: "office.comment",
			want:    true,
		},
		{
			name:    "wildcard does not match multiple segments",
			pattern: "task.*",
			subject: "task.state.changed",
			want:    false,
		},
		{
			name:    "literal mismatch fails",
			pattern: "task.created",
			subject: "task.updated",
			want:    false,
		},
		{
			name:    "different prefix fails even with wildcard",
			pattern: "task.*",
			subject: "office.created",
			want:    false,
		},
		{
			name:    "cross-plugin event wildcard",
			pattern: "plugin.kandev-plugin-jira.*",
			subject: "plugin.kandev-plugin-jira.sync-completed",
			want:    true,
		},
		{
			name:    "cross-plugin event wildcard wrong plugin id",
			pattern: "plugin.kandev-plugin-jira.*",
			subject: "plugin.kandev-plugin-slack.sync-completed",
			want:    false,
		},
		{
			name:    "bare wildcard matches single-segment subject",
			pattern: "*",
			subject: "task",
			want:    true,
		},
		{
			name:    "empty pattern and subject match",
			pattern: "",
			subject: "",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchSubject(tt.pattern, tt.subject)
			if got != tt.want {
				t.Fatalf("MatchSubject(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
			}
		})
	}
}
