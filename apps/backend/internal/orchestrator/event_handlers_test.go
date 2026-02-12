package orchestrator

import "testing"

func TestIsResumeFailure(t *testing.T) {
	tests := []struct {
		name     string
		errorMsg string
		want     bool
	}{
		{
			name:     "exact match",
			errorMsg: "no conversation found",
			want:     true,
		},
		{
			name:     "mixed case",
			errorMsg: "No Conversation Found",
			want:     true,
		},
		{
			name:     "embedded in longer message",
			errorMsg: "prompt failed: no conversation found for session abc-123",
			want:     true,
		},
		{
			name:     "unrelated error",
			errorMsg: "connection refused",
			want:     false,
		},
		{
			name:     "empty string",
			errorMsg: "",
			want:     false,
		},
		{
			name:     "agent crashed",
			errorMsg: "agent process exited with code 1",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isResumeFailure(tt.errorMsg)
			if got != tt.want {
				t.Errorf("isResumeFailure(%q) = %v, want %v", tt.errorMsg, got, tt.want)
			}
		})
	}
}
