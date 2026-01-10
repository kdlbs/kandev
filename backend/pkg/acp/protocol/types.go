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

// ControlData represents control commands for agents (Backend → Agent)
type ControlData struct {
	Action string `json:"action"` // pause, resume, stop, cancel
	Reason string `json:"reason,omitempty"`
}

// InputRequiredData represents a request for user input (Agent → Backend)
type InputRequiredData struct {
	PromptID    string   `json:"prompt_id"`              // Unique ID for this prompt
	Prompt      string   `json:"prompt"`                 // Question/prompt for the user
	InputType   string   `json:"input_type"`             // text, choice, confirm, file
	Options     []string `json:"options,omitempty"`      // For choice type
	Default     string   `json:"default,omitempty"`      // Default value
	Required    bool     `json:"required"`               // Whether input is required
	Timeout     int      `json:"timeout,omitempty"`      // Timeout in seconds (0 = no timeout)
	Context     string   `json:"context,omitempty"`      // Additional context for the prompt
	Validation  string   `json:"validation,omitempty"`   // Validation pattern (regex)
}

// InputResponseData represents user's response to input request (Backend → Agent)
type InputResponseData struct {
	PromptID  string `json:"prompt_id"`           // ID of the prompt being responded to
	Response  string `json:"response"`            // User's response
	Cancelled bool   `json:"cancelled,omitempty"` // User cancelled the input
}

// SessionInfoData represents session information from the agent
type SessionInfoData struct {
	SessionID   string                 `json:"session_id"`
	Resumable   bool                   `json:"resumable"`
	StartedAt   string                 `json:"started_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AgentCapabilities represents what the agent can do
type AgentCapabilities struct {
	SupportsInput   bool     `json:"supports_input"`   // Can handle input_required
	SupportsControl bool     `json:"supports_control"` // Can handle control commands
	SupportsPause   bool     `json:"supports_pause"`   // Can be paused/resumed
	Features        []string `json:"features,omitempty"`
}

// ACPUpdateData represents session update data from ACP protocol
type ACPUpdateData struct {
	Type string                 `json:"type"` // content, toolCall, thinking, error, complete
	Data map[string]interface{} `json:"data,omitempty"`
}

// ACPContentData represents content update from ACP
type ACPContentData struct {
	Text string `json:"text"`
}

// ACPToolCallData represents tool call update from ACP
type ACPToolCallData struct {
	ToolName string                 `json:"tool_name"`
	Args     map[string]interface{} `json:"args,omitempty"`
	Status   string                 `json:"status"` // pending, running, complete, error
	Result   string                 `json:"result,omitempty"`
}

// ACPCompleteData represents completion update from ACP
type ACPCompleteData struct {
	SessionID string `json:"session_id"`
	Success   bool   `json:"success"`
}
