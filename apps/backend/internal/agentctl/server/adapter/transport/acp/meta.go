package acp

// buildSessionMeta returns the agent-specific _meta payload to attach to the
// ACP session/new request. Each supported agent has its own builder; agents
// not listed return nil and the bridge sees no _meta (current behaviour).
//
// This dispatcher is intentionally agent-aware so adapter.go::NewSession stays
// agent-neutral. Adding a new agent's _meta extension means adding a case here
// and a builder file (`meta_<agent>.go`) — no changes to NewSession.
func (a *Adapter) buildSessionMeta() map[string]any {
	switch a.agentID {
	case "claude-acp":
		return buildClaudeCodeMeta(a.cfg.CLIFlagTokens)
	default:
		return nil
	}
}
