package handlers

import (
	"encoding/json"
	"testing"
)

func TestWorkflowHandlerRequestsCarryProfileSessionPolicy(t *testing.T) {
	tests := []struct {
		name string
		make func() any
		want string
	}{
		{name: "http create", make: func() any { return &httpCreateWorkflowRequest{} }, want: "park_reuse"},
		{name: "http update", make: func() any { return &httpUpdateWorkflowRequest{} }, want: "park_new"},
		{name: "websocket create", make: func() any { return &wsCreateWorkflowRequest{} }, want: "park_reuse"},
		{name: "websocket update", make: func() any { return &wsUpdateWorkflowRequest{} }, want: "park_new"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := tt.make()
			input, err := json.Marshal(map[string]string{"profile_session_policy": tt.want})
			if err != nil {
				t.Fatalf("marshal input: %v", err)
			}
			if err := json.Unmarshal(input, request); err != nil {
				t.Fatalf("decode input: %v", err)
			}
			output, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			var fields map[string]any
			if err := json.Unmarshal(output, &fields); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if fields["profile_session_policy"] != tt.want {
				t.Fatalf("profile_session_policy = %v, want %q", fields["profile_session_policy"], tt.want)
			}
		})
	}
}
