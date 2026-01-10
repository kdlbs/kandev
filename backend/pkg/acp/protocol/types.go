package protocol

// ProgressData represents progress update data
type ProgressData struct {
	Progress       int    `json:"progress"`                  // 0-100
	Message        string `json:"message"`
	CurrentFile    string `json:"current_file,omitempty"`
	FilesProcessed int    `json:"files_processed,omitempty"`
	TotalFiles     int    `json:"total_files,omitempty"`
}

// LogData represents log message data
type LogData struct {
	Level    string                 `json:"level"` // debug, info, warn, error
	Message  string                 `json:"message"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ResultData represents task result data
type ResultData struct {
	Status    string     `json:"status"` // completed, failed, cancelled
	Summary   string     `json:"summary"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// Artifact represents a generated file/output
type Artifact struct {
	Type string `json:"type"` // report, code, log
	Path string `json:"path"`
	URL  string `json:"url,omitempty"`
}

// ErrorData represents error message data
type ErrorData struct {
	Error   string `json:"error"`
	File    string `json:"file,omitempty"`
	Details string `json:"details,omitempty"`
}

// StatusData represents agent status data
type StatusData struct {
	Status  string `json:"status"` // started, running, paused, stopped
	Message string `json:"message,omitempty"`
}

// ControlData represents control commands for agents
type ControlData struct {
	Action string `json:"action"` // pause, resume, stop
	Reason string `json:"reason,omitempty"`
}

