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
	// ProtocolOpenCode is the OpenCode CLI protocol (REST/SSE over HTTP).
	// OpenCode spawns an HTTP server and communicates via REST API calls
	// with Server-Sent Events (SSE) for streaming responses.
	ProtocolOpenCode Protocol = "opencode"
	// ProtocolCopilot is the GitHub Copilot SDK protocol.
	// Uses the official Go SDK which internally communicates via JSON-RPC with the Copilot CLI.
	ProtocolCopilot Protocol = "copilot"
	// ProtocolAmp is the Sourcegraph Amp protocol (stream-json over stdin/stdout).
	// Amp uses a streaming JSON format similar to Claude Code, with thread-based
	// session management for multi-turn conversations.
	ProtocolAmp Protocol = "amp"
)

// String returns the string representation of the protocol.
func (p Protocol) String() string {
	return string(p)
}

// IsValid returns true if the protocol is a known valid protocol.
func (p Protocol) IsValid() bool {
	switch p {
	case ProtocolACP, ProtocolREST, ProtocolMCP, ProtocolCodex, ProtocolClaudeCode, ProtocolOpenCode, ProtocolCopilot, ProtocolAmp:
		return true
	default:
		return false
	}
}

