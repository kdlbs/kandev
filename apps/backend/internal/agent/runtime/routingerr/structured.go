package routingerr

import "fmt"

// structuredProviderCode maps only signals whose protocol/provider semantics
// are known. Generic internal RPC errors and API wrapper names are not auth.
func structuredProviderCode(in Input) (Code, string) {
	if in.HTTPStatus >= 400 && in.HTTPStatus <= 599 {
		code := httpStatusToCode(in.HTTPStatus)
		if code == "" {
			code = CodeUnknownProvider
		}
		return code, fmt.Sprintf("http.%d", in.HTTPStatus)
	}
	// ACP defines -32000 as Authentication required. This is not the JSON-RPC
	// internal-error code (-32603), and must never be interpreted as HTTP.
	if in.ACPCode == -32000 {
		return CodeAuthRequired, "acp.authentication_required"
	}
	if in.ProviderID == "opencode-acp" && in.ErrorKind == "ProviderAuthError" {
		return CodeAuthRequired, "opencode.kind.provider_auth"
	}
	// Error kinds are adapter/provider namespaced. Do not interpret an opaque
	// ProviderCode or another provider's identically named kind as this API.
	if in.ProviderID == "claude-acp" {
		switch in.ErrorKind {
		case "authentication_error":
			return CodeAuthRequired, "anthropic.kind.authentication"
		case "rate_limit_error":
			return CodeRateLimited, "anthropic.kind.rate_limit"
		case "overloaded_error":
			return CodeProviderOverloaded, "anthropic.kind.overloaded"
		case "api_error":
			return CodeProviderUnavailable, "anthropic.kind.api"
		}
	}
	return "", ""
}
