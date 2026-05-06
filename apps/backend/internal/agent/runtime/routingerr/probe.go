package routingerr

import (
	"context"
	"sync"
)

// ProbeInput is the routing-aware payload supplied to a ProviderProber.
type ProbeInput struct {
	ProviderID  string
	WorkspaceID string
	Model       string
}

// ProviderProber is the optional cheap-availability check for a provider.
// Implementations should complete in under five seconds and must not start
// an agent session. Returns a non-nil *Error when the probe fails so the
// caller can reuse the classifier+sanitizer paths uniformly.
type ProviderProber interface {
	Probe(ctx context.Context, in ProbeInput) *Error
}

var (
	proberMu sync.RWMutex
	probers  = map[string]ProviderProber{}
)

// RegisterProber wires a ProviderProber to a provider ID. Re-registering
// the same provider overwrites the previous prober (last-write-wins).
func RegisterProber(providerID string, p ProviderProber) {
	proberMu.Lock()
	defer proberMu.Unlock()
	probers[providerID] = p
}

// GetProber returns the registered prober for a provider ID, if any. The
// scheduler treats a missing prober as "use the next real launch as the
// probe" — no default LaunchAsProbe shim lives here in v1.
func GetProber(providerID string) (ProviderProber, bool) {
	proberMu.RLock()
	defer proberMu.RUnlock()
	p, ok := probers[providerID]
	return p, ok
}
