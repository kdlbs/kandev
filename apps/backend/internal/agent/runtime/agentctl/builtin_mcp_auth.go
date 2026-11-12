package client

import "sync/atomic"

// builtinMCPAuthEnforced is the effective features.builtinMcpCallerAuth value
// for this backend process. The toggle is restart-required, so one value is
// applied to every instance this process creates, through every executor.
var builtinMCPAuthEnforced atomic.Bool

// SetBuiltinMCPAuthEnforced records the effective built-in MCP caller-auth
// toggle. Backend startup calls it once, before any instance is created.
func SetBuiltinMCPAuthEnforced(enforced bool) {
	builtinMCPAuthEnforced.Store(enforced)
}

// BuiltinMCPAuthEnforced reports the value CreateInstance sends as
// builtin_mcp_auth_enforced.
func BuiltinMCPAuthEnforced() bool {
	return builtinMCPAuthEnforced.Load()
}
