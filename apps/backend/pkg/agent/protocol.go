// Package agent provides shared types for agent communication.
package agent

// Protocol defines the communication protocol used by an agent.
type Protocol string

const (
	// ProtocolACP is the Agent Communication Protocol (JSON-RPC over stdin/stdout).
	ProtocolACP Protocol = "acp"
	// ProtocolREST is for agents with REST APIs.
	ProtocolREST Protocol = "rest"
	// ProtocolMCP is the Model Context Protocol.
	ProtocolMCP Protocol = "mcp"
	// ProtocolCodex is the OpenAI Codex app-server protocol (JSON-RPC variant over stdin/stdout).
	// Codex uses a similar structure to ACP but with different message formats and
	// a Thread/Turn model instead of Session/Prompt.
	ProtocolCodex Protocol = "codex"
	// ProtocolClaudeCode is the Claude Code CLI protocol (stream-json over stdin/stdout).
	// Claude Code uses a streaming JSON format with control requests for permissions
	// and user messages for prompts.
	ProtocolClaudeCode Protocol = "claude-code"
)

// String returns the string representation of the protocol.
func (p Protocol) String() string {
	return string(p)
}

// IsValid returns true if the protocol is a known valid protocol.
func (p Protocol) IsValid() bool {
	switch p {
	case ProtocolACP, ProtocolREST, ProtocolMCP, ProtocolCodex, ProtocolClaudeCode:
		return true
	default:
		return false
	}
}

