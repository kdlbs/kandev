package registry

// RoutableProviderIDs is the canonical, single-source-of-truth list of
// provider IDs eligible for office provider routing in v1. It lives in
// the registry package so both:
//
//  1. internal/office/routing's catalogue (KnownProviders), and
//  2. the E2E multi-mock registration path (KANDEV_MOCK_PROVIDERS),
//
// can reference the same list without an import cycle. Adding a new
// real CLI provider to routing v2+ means appending its agent-registry
// ID here.
//
// These IDs MUST match the corresponding real agents' ID() values
// (claude_acp.go, codex_acp.go, opencode_acp.go, copilot_acp.go,
// amp_acp.go) so registry lookups against an enabled real agent
// succeed.
var RoutableProviderIDs = []string{
	"claude-acp",
	"codex-acp",
	"opencode-acp",
	"copilot-acp",
	"amp-acp",
}

// IsRoutableProviderID reports whether id is in RoutableProviderIDs.
func IsRoutableProviderID(id string) bool {
	for _, p := range RoutableProviderIDs {
		if p == id {
			return true
		}
	}
	return false
}
