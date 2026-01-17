package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorkspaceMessageType_Constants(t *testing.T) {
	// Verify message type constants are defined correctly
	tests := []struct {
		name     string
		msgType  WorkspaceMessageType
		expected string
	}{
		{"ShellOutput", WorkspaceMessageTypeShellOutput, "shell_output"},
		{"ShellInput", WorkspaceMessageTypeShellInput, "shell_input"},
		{"ShellExit", WorkspaceMessageTypeShellExit, "shell_exit"},
		{"Ping", WorkspaceMessageTypePing, "ping"},
		{"Pong", WorkspaceMessageTypePong, "pong"},
		{"GitStatus", WorkspaceMessageTypeGitStatus, "git_status"},
		{"FileChange", WorkspaceMessageTypeFileChange, "file_change"},
		{"FileList", WorkspaceMessageTypeFileList, "file_list"},
		{"Error", WorkspaceMessageTypeError, "error"},
		{"Connected", WorkspaceMessageTypeConnected, "connected"},
		{"ShellResize", WorkspaceMessageTypeShellResize, "shell_resize"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.msgType) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.msgType)
			}
		})
	}
}

func TestNewWorkspaceShellOutput(t *testing.T) {
	data := "Hello, World!"
	msg := NewWorkspaceShellOutput(data)

	if msg.Type != WorkspaceMessageTypeShellOutput {
		t.Errorf("expected type %q, got %q", WorkspaceMessageTypeShellOutput, msg.Type)
	}
	if msg.Data != data {
		t.Errorf("expected data %q, got %q", data, msg.Data)
	}
	if msg.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewWorkspaceShellInput(t *testing.T) {
	data := "ls -la\n"
	msg := NewWorkspaceShellInput(data)

	if msg.Type != WorkspaceMessageTypeShellInput {
		t.Errorf("expected type %q, got %q", WorkspaceMessageTypeShellInput, msg.Type)
	}
	if msg.Data != data {
		t.Errorf("expected data %q, got %q", data, msg.Data)
	}
}

func TestNewWorkspaceGitStatus(t *testing.T) {
	update := &GitStatusUpdate{
		Branch:    "main",
		Ahead:     1,
		Behind:    2,
		Timestamp: time.Now(),
	}
	msg := NewWorkspaceGitStatus(update)

	if msg.Type != WorkspaceMessageTypeGitStatus {
		t.Errorf("expected type %q, got %q", WorkspaceMessageTypeGitStatus, msg.Type)
	}
	if msg.GitStatus == nil {
		t.Fatal("expected non-nil GitStatus")
	}
	if msg.GitStatus.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", msg.GitStatus.Branch)
	}
}

func TestNewWorkspaceFileChange(t *testing.T) {
	notification := &FileChangeNotification{
		Path:      "/src/main.go",
		Operation: "modify",
		Timestamp: time.Now(),
	}
	msg := NewWorkspaceFileChange(notification)

	if msg.Type != WorkspaceMessageTypeFileChange {
		t.Errorf("expected type %q, got %q", WorkspaceMessageTypeFileChange, msg.Type)
	}
	if msg.FileChange == nil {
		t.Fatal("expected non-nil FileChange")
	}
	if msg.FileChange.Path != "/src/main.go" {
		t.Errorf("expected path '/src/main.go', got %q", msg.FileChange.Path)
	}
}

func TestNewWorkspaceConnected(t *testing.T) {
	msg := NewWorkspaceConnected()

	if msg.Type != WorkspaceMessageTypeConnected {
		t.Errorf("expected type %q, got %q", WorkspaceMessageTypeConnected, msg.Type)
	}
	if msg.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewWorkspacePong(t *testing.T) {
	msg := NewWorkspacePong()

	if msg.Type != WorkspaceMessageTypePong {
		t.Errorf("expected type %q, got %q", WorkspaceMessageTypePong, msg.Type)
	}
}

func TestWorkspaceStreamMessage_JSONSerialization(t *testing.T) {
	msg := NewWorkspaceShellOutput("test output")

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded WorkspaceStreamMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("expected type %q, got %q", msg.Type, decoded.Type)
	}
	if decoded.Data != msg.Data {
		t.Errorf("expected data %q, got %q", msg.Data, decoded.Data)
	}
}

