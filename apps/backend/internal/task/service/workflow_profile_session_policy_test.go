package service

import (
	"encoding/json"
	"testing"
)

func TestWorkflowRequestsCarryProfileSessionPolicy(t *testing.T) {
	tests := []struct {
		name string
		make func() (any, string)
	}{
		{
			name: "create",
			make: func() (any, string) {
				return &CreateWorkflowRequest{}, "park_reuse"
			},
		},
		{
			name: "update",
			make: func() (any, string) {
				return &UpdateWorkflowRequest{}, "park_new"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, want := tt.make()
			input, err := json.Marshal(map[string]string{"profile_session_policy": want})
			if err != nil {
				t.Fatalf("marshal request input: %v", err)
			}
			if err := json.Unmarshal(input, request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			output, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("marshal decoded request: %v", err)
			}
			var fields map[string]any
			if err := json.Unmarshal(output, &fields); err != nil {
				t.Fatalf("decode marshaled request: %v", err)
			}
			if fields["profile_session_policy"] != want {
				t.Fatalf("profile_session_policy = %v, want %q", fields["profile_session_policy"], want)
			}
		})
	}
}
