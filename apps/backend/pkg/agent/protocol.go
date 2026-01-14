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
)

// String returns the string representation of the protocol.
func (p Protocol) String() string {
	return string(p)
}

// IsValid returns true if the protocol is a known valid protocol.
func (p Protocol) IsValid() bool {
	switch p {
	case ProtocolACP, ProtocolREST, ProtocolMCP:
		return true
	default:
		return false
	}
}

