package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestIsManagedConversationTask(t *testing.T) {
	tests := []struct {
		name string
		task *models.Task
		want bool
	}{
		{name: "nil task", task: nil, want: false},
		{name: "nil metadata", task: &models.Task{IsEphemeral: true}, want: false},
		{
			name: "ephemeral with plugin id",
			task: &models.Task{
				IsEphemeral: true,
				Metadata:    map[string]interface{}{metaKeyPluginID: "plugin-coordinator"},
			},
			want: true,
		},
		{
			name: "non-ephemeral with plugin id",
			task: &models.Task{
				Metadata: map[string]interface{}{metaKeyPluginID: "plugin-coordinator"},
			},
			want: false,
		},
		{
			name: "stale ephemeral column falls back to metadata",
			task: &models.Task{
				Metadata: map[string]interface{}{
					metaKeyPluginID:  "plugin-coordinator",
					metaKeyEphemeral: true,
				},
			},
			want: true,
		},
		{
			name: "ephemeral without plugin id",
			task: &models.Task{
				IsEphemeral: true,
				Metadata:    map[string]interface{}{"some_other_key": "value"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsManagedConversationTask(tt.task); got != tt.want {
				t.Fatalf("IsManagedConversationTask = %v, want %v", got, tt.want)
			}
		})
	}
}
