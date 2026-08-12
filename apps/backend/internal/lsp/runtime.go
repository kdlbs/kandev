package lsp

import (
	"encoding/json"
	"time"
)

type Activity string

const (
	ActivityIdle       Activity = "idle"
	ActivityServerWork Activity = "server_work"
)

type WorkItem struct {
	Token      string    `json:"token"`
	Title      string    `json:"title"`
	Message    string    `json:"message,omitempty"`
	Percentage *float64  `json:"percentage,omitempty"`
	StartedAt  time.Time `json:"started_at"`
}

// CompletedWorkItem is the last server-reported work token that ended for a
// live generation. It is completion evidence, not active progress.
type CompletedWorkItem struct {
	Token       string    `json:"token"`
	Title       string    `json:"title"`
	Message     string    `json:"message,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// WorkspaceUpdateResult reports which live languages accepted an in-place
// root update and which must keep their old scope until explicit restart.
type WorkspaceUpdateResult struct {
	WorkspaceFolders         []WorkspaceFolder `json:"workspace_folders"`
	DynamicLanguages         []string          `json:"dynamic_languages"`
	RestartRequiredLanguages []string          `json:"restart_required_languages"`
}

type RuntimeSnapshot struct {
	Language            string             `json:"language"`
	Generation          uint64             `json:"generation"`
	Revision            uint64             `json:"revision"`
	Incarnation         string             `json:"incarnation,omitempty"`
	RuntimeStartedAt    time.Time          `json:"runtime_started_at,omitempty"`
	Phase               Phase              `json:"phase"`
	Activity            Activity           `json:"activity"`
	ProcessStartedAt    *time.Time         `json:"process_started_at,omitempty"`
	InitializeStartedAt *time.Time         `json:"initialize_started_at,omitempty"`
	ReadyAt             *time.Time         `json:"ready_at,omitempty"`
	LastTransitionAt    time.Time          `json:"last_transition_at"`
	Work                []WorkItem         `json:"work"`
	LastCompletedWork   *CompletedWorkItem `json:"last_completed_work,omitempty"`
	Capabilities        json.RawMessage    `json:"capabilities,omitempty"`
	Diagnostics         []json.RawMessage  `json:"diagnostics,omitempty"`
	ErrorCode           string             `json:"error_code,omitempty"`
	ErrorMessage        string             `json:"error_message,omitempty"`
	WorkspacePath       string             `json:"workspace_path"`
	WorkspaceURI        string             `json:"workspace_uri"`
	WorkspaceFolders    []WorkspaceFolder  `json:"workspace_folders"`
}

type TaskHostStartRequest struct {
	Language      string          `json:"language"`
	Generation    uint64          `json:"generation"`
	AutoInstall   bool            `json:"auto_install"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

type TaskHostConfigurationRequest struct {
	Language      string          `json:"language"`
	Generation    uint64          `json:"generation"`
	Configuration json.RawMessage `json:"configuration"`
}

type TaskHostStopRequest struct {
	Language   string `json:"language"`
	Generation uint64 `json:"generation,omitempty"`
	Reason     string `json:"reason,omitempty"`
}
