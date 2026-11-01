package backendapp

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// routingCircuitSnapshots is the read-only slice of the dynamic runtime the
// settings surface depends on. The resolver satisfies it; no routing method
// is reachable through this seam.
type routingCircuitSnapshots interface {
	CircuitSnapshots() []dynamic.CircuitSnapshot
}

// routingCircuitReader maps engine circuits onto the settings wire shape. The
// resource key stays the router's own key; scope and scope_value are its two
// colon-separated halves, so the client never parses a key itself.
type routingCircuitReader struct {
	source routingCircuitSnapshots
}

func newRoutingCircuitReader(source routingCircuitSnapshots) routingCircuitReader {
	return routingCircuitReader{source: source}
}

func (r routingCircuitReader) ListRoutingCircuits(context.Context) ([]controller.RoutingCircuit, error) {
	snapshots := r.source.CircuitSnapshots()
	circuits := make([]controller.RoutingCircuit, 0, len(snapshots))
	for _, snapshot := range snapshots {
		scope, value := splitRoutingResourceKey(snapshot.Key)
		circuits = append(circuits, controller.RoutingCircuit{
			ResourceKey: snapshot.Key,
			Scope:       scope,
			ScopeValue:  value,
			State:       string(snapshot.State),
			Code:        string(snapshot.Code),
			Until:       utcOrNil(snapshot.Until),
			ProbeUntil:  utcOrNil(snapshot.ProbeUntil),
			Failures:    snapshot.Failures,
			UpdatedAt:   utcOrNil(snapshot.UpdatedAt),
		})
	}
	return circuits, nil
}

func splitRoutingResourceKey(key string) (string, string) {
	scope, value, ok := strings.Cut(key, ":")
	if !ok {
		return "", key
	}
	return scope, value
}

func utcOrNil(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	utc := at.UTC()
	return &utc
}
