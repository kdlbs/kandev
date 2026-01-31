package streams

import (
	"encoding/json"
	"testing"
)

func TestNormalizedPayloadMarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload *NormalizedPayload
	}{
		{
			name:    "shell_exec",
			payload: NewShellExec("ls -la", "/home/user", "list files", 30000, false),
		},
		{
			name:    "read_file",
			payload: NewReadFile("/path/to/file.go", 0, 100),
		},
		{
			name:    "modify_file",
			payload: NewModifyFile("/path/to/file.go", []FileMutation{{Type: MutationPatch, Diff: "- old\n+ new"}}),
		},
		{
			name:    "code_search",
			payload: NewCodeSearch("query", "pattern", "/path", "*.go"),
		},
		{
			name:    "generic",
			payload: NewGeneric("custom_tool", map[string]any{"key": "value"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal
			var result NormalizedPayload
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Check kind matches
			if result.Kind() != tt.payload.Kind() {
				t.Errorf("Kind mismatch: got %q, want %q", result.Kind(), tt.payload.Kind())
			}

			// Check specific fields based on kind
			switch tt.payload.Kind() {
			case ToolKindShellExec:
				if result.ShellExec() == nil {
					t.Error("ShellExec() is nil after unmarshal")
				} else if result.ShellExec().Command != tt.payload.ShellExec().Command {
					t.Errorf("ShellExec.Command mismatch: got %q, want %q",
						result.ShellExec().Command, tt.payload.ShellExec().Command)
				}
			case ToolKindReadFile:
				if result.ReadFile() == nil {
					t.Error("ReadFile() is nil after unmarshal")
				} else if result.ReadFile().FilePath != tt.payload.ReadFile().FilePath {
					t.Errorf("ReadFile.FilePath mismatch: got %q, want %q",
						result.ReadFile().FilePath, tt.payload.ReadFile().FilePath)
				}
			case ToolKindModifyFile:
				if result.ModifyFile() == nil {
					t.Error("ModifyFile() is nil after unmarshal")
				} else if result.ModifyFile().FilePath != tt.payload.ModifyFile().FilePath {
					t.Errorf("ModifyFile.FilePath mismatch: got %q, want %q",
						result.ModifyFile().FilePath, tt.payload.ModifyFile().FilePath)
				}
			case ToolKindGeneric:
				if result.Generic() == nil {
					t.Error("Generic() is nil after unmarshal")
				} else if result.Generic().Name != tt.payload.Generic().Name {
					t.Errorf("Generic.Name mismatch: got %q, want %q",
						result.Generic().Name, tt.payload.Generic().Name)
				}
			}
		})
	}
}
