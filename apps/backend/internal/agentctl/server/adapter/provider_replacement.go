package adapter

import "context"

// ProviderMCPReplacer proves that a native conversation can survive a provider
// process replacement without sending a prompt. Unsupported transports do not
// implement this interface.
type ProviderMCPReplacer interface {
	ProviderMCPReplacementReady(context.Context) error
	ProviderMCPCatalog(context.Context, string) (map[string][]string, error)
}
