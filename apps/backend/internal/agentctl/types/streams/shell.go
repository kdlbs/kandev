package streams

// Shell message type constants.
const (
	// ShellMsgTypeInput indicates user input to the shell.
	ShellMsgTypeInput = "input"

	// ShellMsgTypeOutput indicates output from the shell.
	ShellMsgTypeOutput = "output"

	// ShellMsgTypePing is a ping message for keepalive.
	ShellMsgTypePing = "ping"

	// ShellMsgTypePong is a pong response to ping.
	ShellMsgTypePong = "pong"

	// ShellMsgTypeExit indicates the shell has exited.
	ShellMsgTypeExit = "exit"
)

// ShellMessage is the message type for bidirectional shell stream.
// Used for interactive shell I/O over WebSocket.
//
// Stream endpoint: ws://.../api/v1/shell/stream
type ShellMessage struct {
	// Type indicates the message type. Use ShellMsgType* constants:
	// "input", "output", "ping", "pong", "exit".
	Type string `json:"type"`

	// Data contains the shell input or output data.
	Data string `json:"data,omitempty"`

	// Code is the exit code (for "exit" type only).
	Code int `json:"code,omitempty"`
}

// ShellStatusResponse is the response from the shell status endpoint.
//
// HTTP endpoint: GET /api/v1/shell/status
type ShellStatusResponse struct {
	// Running indicates if the shell is currently running.
	Running bool `json:"running"`

	// Pid is the process ID of the shell.
	Pid int `json:"pid"`

	// Shell is the shell type (e.g., "/bin/bash").
	Shell string `json:"shell"`

	// Cwd is the current working directory.
	Cwd string `json:"cwd"`

	// StartedAt is when the shell was started (ISO 8601 format).
	StartedAt string `json:"started_at"`
}

// ShellBufferResponse is the response from the shell buffer endpoint.
//
// HTTP endpoint: GET /api/v1/shell/buffer
type ShellBufferResponse struct {
	// Data contains the buffered shell output.
	Data string `json:"data"`
}
