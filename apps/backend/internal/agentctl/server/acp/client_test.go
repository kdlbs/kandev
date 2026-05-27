package acp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestResolvePath(t *testing.T) {
	client := NewClient(WithWorkspaceRoot("/workspace/project"))

	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{
			name:     "absolute path within workspace",
			input:    "/workspace/project/src/main.go",
			expected: "/workspace/project/src/main.go",
		},
		{
			name:     "relative path resolves within workspace",
			input:    "src/main.go",
			expected: filepath.Join("/workspace/project", "src/main.go"),
		},
		{
			name:     "workspace root itself is allowed",
			input:    "/workspace/project",
			expected: "/workspace/project",
		},
		{
			name:     "dot path resolves to workspace root",
			input:    ".",
			expected: "/workspace/project",
		},
		{
			name:      "path traversal with relative path is rejected",
			input:     "../../etc/passwd",
			expectErr: true,
		},
		{
			name:      "path traversal with dot-dot in middle is rejected",
			input:     "src/../../etc/passwd",
			expectErr: true,
		},
		{
			name:      "absolute path outside workspace is rejected",
			input:     "/etc/passwd",
			expectErr: true,
		},
		{
			name:      "absolute path with parent traversal is rejected",
			input:     "/workspace/project/../../../etc/passwd",
			expectErr: true,
		},
		{
			name:     "nested relative path within workspace",
			input:    "src/pkg/handler.go",
			expected: filepath.Join("/workspace/project", "src/pkg/handler.go"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.resolvePath(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Errorf("resolvePath(%q) expected error, got path %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("resolvePath(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.expected {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestForwardPermissionRequestTitleDerivation(t *testing.T) {
	str := func(s string) *string { return &s }
	kind := func(k acp.ToolKind) *acp.ToolKind { return &k }
	allowOpt := acp.PermissionOption{OptionId: "allow", Name: "Allow", Kind: acp.PermissionOptionKindAllowOnce}

	tests := []struct {
		name           string
		toolCall       acp.ToolCallUpdate
		wantTitle      string
		wantActionType string
		wantDescInDtl  bool
	}{
		{
			name: "title present, kind other -> human title wins",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: "tc-1",
				Title:      str("Run bash command 'ls -la'"),
				Kind:       kind(acp.ToolKindOther),
			},
			wantTitle:      "Run bash command 'ls -la'",
			wantActionType: "other",
			wantDescInDtl:  false, // description == title, so not duplicated
		},
		{
			name: "title present, kind execute -> human title still wins",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: "tc-2",
				Title:      str("Run bash command 'rm -rf'"),
				Kind:       kind(acp.ToolKindExecute),
			},
			wantTitle:      "Run bash command 'rm -rf'",
			wantActionType: "execute",
			wantDescInDtl:  false,
		},
		{
			name: "only kind -> kind is the title",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: "tc-3",
				Kind:       kind(acp.ToolKindEdit),
			},
			wantTitle:      "edit",
			wantActionType: "edit",
			wantDescInDtl:  false,
		},
		{
			name: "only title, no kind",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: "tc-4",
				Title:      str("Reading configuration file"),
			},
			wantTitle:      "Reading configuration file",
			wantActionType: "",
			wantDescInDtl:  false,
		},
		{
			name: "neither title nor kind -> both empty",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: "tc-5",
			},
			wantTitle:      "",
			wantActionType: "",
			wantDescInDtl:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient()
			var captured *types.PermissionRequest
			handler := func(_ context.Context, req *types.PermissionRequest) (*types.PermissionResponse, error) {
				captured = req
				return &types.PermissionResponse{OptionID: "allow"}, nil
			}
			req := acp.RequestPermissionRequest{
				SessionId: "sess",
				ToolCall:  tt.toolCall,
				Options:   []acp.PermissionOption{allowOpt},
			}
			if _, err := c.forwardPermissionRequest(context.Background(), handler, req); err != nil {
				t.Fatalf("forwardPermissionRequest returned error: %v", err)
			}
			if captured == nil {
				t.Fatal("handler not invoked")
			}
			if captured.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", captured.Title, tt.wantTitle)
			}
			if captured.ActionType != tt.wantActionType {
				t.Errorf("ActionType = %q, want %q", captured.ActionType, tt.wantActionType)
			}
			_, hasDesc := captured.ActionDetails["description"]
			if hasDesc != tt.wantDescInDtl {
				t.Errorf("ActionDetails.description present = %v, want %v (details=%v)", hasDesc, tt.wantDescInDtl, captured.ActionDetails)
			}
		})
	}
}
