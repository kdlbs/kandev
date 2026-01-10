package protocol

import (
	"testing"
)

func TestNewProgressMessage(t *testing.T) {
	data := ProgressData{
		Progress:       50,
		Message:        "Processing files",
		CurrentFile:    "main.go",
		FilesProcessed: 5,
		TotalFiles:     10,
	}

	msg := NewProgressMessage("agent-1", "task-1", data)

	if msg.Type != MessageTypeProgress {
		t.Errorf("Expected type %s, got %s", MessageTypeProgress, msg.Type)
	}
	if msg.AgentID != "agent-1" {
		t.Errorf("Expected agentID 'agent-1', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-1" {
		t.Errorf("Expected taskID 'task-1', got %s", msg.TaskID)
	}
	if msg.Data["progress"] != 50 {
		t.Errorf("Expected progress 50, got %v", msg.Data["progress"])
	}
	if msg.Data["message"] != "Processing files" {
		t.Errorf("Expected message 'Processing files', got %v", msg.Data["message"])
	}
	if msg.Data["current_file"] != "main.go" {
		t.Errorf("Expected current_file 'main.go', got %v", msg.Data["current_file"])
	}
	if msg.Data["files_processed"] != 5 {
		t.Errorf("Expected files_processed 5, got %v", msg.Data["files_processed"])
	}
	if msg.Data["total_files"] != 10 {
		t.Errorf("Expected total_files 10, got %v", msg.Data["total_files"])
	}
}

func TestNewLogMessage(t *testing.T) {
	data := LogData{
		Level:   "info",
		Message: "Task started",
		Metadata: map[string]interface{}{
			"component": "scanner",
		},
	}

	msg := NewLogMessage("agent-2", "task-2", data)

	if msg.Type != MessageTypeLog {
		t.Errorf("Expected type %s, got %s", MessageTypeLog, msg.Type)
	}
	if msg.AgentID != "agent-2" {
		t.Errorf("Expected agentID 'agent-2', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-2" {
		t.Errorf("Expected taskID 'task-2', got %s", msg.TaskID)
	}
	if msg.Data["level"] != "info" {
		t.Errorf("Expected level 'info', got %v", msg.Data["level"])
	}
	if msg.Data["message"] != "Task started" {
		t.Errorf("Expected message 'Task started', got %v", msg.Data["message"])
	}
	metadata, ok := msg.Data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected metadata to be a map")
	}
	if metadata["component"] != "scanner" {
		t.Errorf("Expected component 'scanner', got %v", metadata["component"])
	}
}

func TestNewResultMessage(t *testing.T) {
	data := ResultData{
		Status:  "completed",
		Summary: "All checks passed",
		Artifacts: []Artifact{
			{Type: "report", Path: "/reports/scan.json", URL: "https://example.com/report"},
			{Type: "log", Path: "/logs/scan.log", URL: ""},
		},
	}

	msg := NewResultMessage("agent-3", "task-3", data)

	if msg.Type != MessageTypeResult {
		t.Errorf("Expected type %s, got %s", MessageTypeResult, msg.Type)
	}
	if msg.AgentID != "agent-3" {
		t.Errorf("Expected agentID 'agent-3', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-3" {
		t.Errorf("Expected taskID 'task-3', got %s", msg.TaskID)
	}
	if msg.Data["status"] != "completed" {
		t.Errorf("Expected status 'completed', got %v", msg.Data["status"])
	}
	if msg.Data["summary"] != "All checks passed" {
		t.Errorf("Expected summary 'All checks passed', got %v", msg.Data["summary"])
	}

	artifacts, ok := msg.Data["artifacts"].([]interface{})
	if !ok {
		t.Fatal("Expected artifacts to be a slice")
	}
	if len(artifacts) != 2 {
		t.Errorf("Expected 2 artifacts, got %d", len(artifacts))
	}

	artifact1, ok := artifacts[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected first artifact to be a map")
	}
	if artifact1["type"] != "report" {
		t.Errorf("Expected first artifact type 'report', got %v", artifact1["type"])
	}
	if artifact1["path"] != "/reports/scan.json" {
		t.Errorf("Expected first artifact path '/reports/scan.json', got %v", artifact1["path"])
	}
}

func TestNewErrorMessage(t *testing.T) {
	data := ErrorData{
		Error:   "Failed to parse file",
		File:    "config.yaml",
		Details: "Invalid YAML syntax on line 42",
	}

	msg := NewErrorMessage("agent-4", "task-4", data)

	if msg.Type != MessageTypeError {
		t.Errorf("Expected type %s, got %s", MessageTypeError, msg.Type)
	}
	if msg.AgentID != "agent-4" {
		t.Errorf("Expected agentID 'agent-4', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-4" {
		t.Errorf("Expected taskID 'task-4', got %s", msg.TaskID)
	}
	if msg.Data["error"] != "Failed to parse file" {
		t.Errorf("Expected error 'Failed to parse file', got %v", msg.Data["error"])
	}
	if msg.Data["file"] != "config.yaml" {
		t.Errorf("Expected file 'config.yaml', got %v", msg.Data["file"])
	}
	if msg.Data["details"] != "Invalid YAML syntax on line 42" {
		t.Errorf("Expected details, got %v", msg.Data["details"])
	}
}

func TestNewStatusMessage(t *testing.T) {
	data := StatusData{
		Status:  "running",
		Message: "Processing task",
	}

	msg := NewStatusMessage("agent-5", "task-5", data)

	if msg.Type != MessageTypeStatus {
		t.Errorf("Expected type %s, got %s", MessageTypeStatus, msg.Type)
	}
	if msg.Data["status"] != "running" {
		t.Errorf("Expected status 'running', got %v", msg.Data["status"])
	}
	if msg.Data["message"] != "Processing task" {
		t.Errorf("Expected message 'Processing task', got %v", msg.Data["message"])
	}
}

func TestNewHeartbeatMessage(t *testing.T) {
	msg := NewHeartbeatMessage("agent-6", "task-6")

	if msg.Type != MessageTypeHeartbeat {
		t.Errorf("Expected type %s, got %s", MessageTypeHeartbeat, msg.Type)
	}
	if msg.AgentID != "agent-6" {
		t.Errorf("Expected agentID 'agent-6', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-6" {
		t.Errorf("Expected taskID 'task-6', got %s", msg.TaskID)
	}
	if len(msg.Data) != 0 {
		t.Errorf("Expected empty data map, got %v", msg.Data)
	}
}

func TestNewControlMessage(t *testing.T) {
	data := ControlData{
		Action: "pause",
		Reason: "User requested pause",
	}

	msg := NewControlMessage("agent-7", "task-7", data)

	if msg.Type != MessageTypeControl {
		t.Errorf("Expected type %s, got %s", MessageTypeControl, msg.Type)
	}
	if msg.AgentID != "agent-7" {
		t.Errorf("Expected agentID 'agent-7', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-7" {
		t.Errorf("Expected taskID 'task-7', got %s", msg.TaskID)
	}
	if msg.Data["action"] != "pause" {
		t.Errorf("Expected action 'pause', got %v", msg.Data["action"])
	}
	if msg.Data["reason"] != "User requested pause" {
		t.Errorf("Expected reason 'User requested pause', got %v", msg.Data["reason"])
	}
}

func TestNewProgressMessage_ZeroValues(t *testing.T) {
	data := ProgressData{
		Progress: 0,
		Message:  "",
	}

	msg := NewProgressMessage("agent-1", "task-1", data)

	if msg.Type != MessageTypeProgress {
		t.Errorf("Expected type %s, got %s", MessageTypeProgress, msg.Type)
	}
	if msg.Data["progress"] != 0 {
		t.Errorf("Expected progress 0, got %v", msg.Data["progress"])
	}
	if msg.Data["message"] != "" {
		t.Errorf("Expected empty message, got %v", msg.Data["message"])
	}
}

func TestNewResultMessage_NoArtifacts(t *testing.T) {
	data := ResultData{
		Status:    "completed",
		Summary:   "No files generated",
		Artifacts: []Artifact{},
	}

	msg := NewResultMessage("agent-1", "task-1", data)

	if msg.Type != MessageTypeResult {
		t.Errorf("Expected type %s, got %s", MessageTypeResult, msg.Type)
	}
	artifacts, ok := msg.Data["artifacts"].([]interface{})
	if !ok {
		t.Fatal("Expected artifacts to be a slice")
	}
	if len(artifacts) != 0 {
		t.Errorf("Expected 0 artifacts, got %d", len(artifacts))
	}
}

func TestNewLogMessage_AllLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			data := LogData{
				Level:   level,
				Message: "Test message",
			}

			msg := NewLogMessage("agent-1", "task-1", data)

			if msg.Data["level"] != level {
				t.Errorf("Expected level '%s', got %v", level, msg.Data["level"])
			}
		})
	}
}

