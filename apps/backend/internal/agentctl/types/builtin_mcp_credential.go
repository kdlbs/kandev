package types

// BuiltinMCPCredentialPath is the control-authenticated instance route that
// returns the instance's built-in MCP caller credential.
const BuiltinMCPCredentialPath = "/api/v1/mcp/builtin-credential"

// BuiltinMCPCredentialResponse is the control-authenticated read of an
// instance's built-in MCP caller credential. The backend uses it to build the
// MCP entries it writes for passthrough and project-file delivery. An older
// agentctl has no such route and enforces no credential.
type BuiltinMCPCredentialResponse struct {
	Credential string `json:"credential"`
	Enforced   bool   `json:"enforced"`
}
