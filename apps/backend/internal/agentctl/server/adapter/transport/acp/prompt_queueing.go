package acp

import "github.com/coder/acp-go-sdk"

// promptQueueingMetaKey is the vendor `_meta` key an agent sets on its ACP
// initialize response to advertise that it accepts a `session/prompt` while
// another prompt for the same session is still in flight.
//
// Observed on `@agentclientprotocol/claude-agent-acp` as
// `agentCapabilities._meta.claudeCode.promptQueueing: true`.
const promptQueueingMetaKey = "promptQueueing"

// agentAdvertisesPromptQueueing reports whether the connected agent advertised
// that a second `session/prompt` may be issued while one is still in flight.
// This is the negotiated precondition for both prompt handoff and mid-turn
// steering.
//
// It is deliberately keyed off the advertisement rather than the agent's
// identity. ADR 0049 rejects a central agent-name whitelist because "an agent
// identity does not prove that the installed adapter/provider version emits the
// lifecycle frames Kandev expects" — a bridge too old to advertise the
// capability must be ineligible even when its name matches.
//
// Note carefully what this does and does not assert. It asserts the agent
// accepts the concurrent prompt. It does NOT assert the agent will fold that
// prompt into the running turn: that decision belongs to the agent CLI beneath
// the bridge, is not advertised over ACP, and its version is not observable
// here. Mid-turn delivery is therefore opportunistic, and a caller must be
// correct whether the agent folds the prompt or runs it as the next turn. See
// docs/specs/platform/requirements/mid-turn-steering.md.
//
// Every malformed shape fails closed to false: absent `_meta`, absent
// namespace, a namespace that is not an object, a missing key, or a value that
// is not a bool.
func agentAdvertisesPromptQueueing(caps acp.AgentCapabilities) bool {
	cc := claudeCodeMeta(caps.Meta)
	if cc == nil {
		return false
	}
	queueing, ok := cc[promptQueueingMetaKey].(bool)
	return ok && queueing
}
