package controller

import (
	"context"
	"errors"
	"time"
)

// ErrRoutingCircuitsUnavailable means the dynamic routing runtime that holds
// the circuits is not wired, so there is no read to answer with.
var ErrRoutingCircuitsUnavailable = errors.New("routing circuits are unavailable")

// RoutingCircuit is one dynamic-routing resource circuit as a read surface
// shows it. ResourceKey is the router's own key; the settings layer splits it
// into scope and scope_value for display. It carries no credential, prompt, or
// log content.
type RoutingCircuit struct {
	ResourceKey string     `json:"resource_key"`
	Scope       string     `json:"scope"`
	ScopeValue  string     `json:"scope_value"`
	State       string     `json:"state"`
	Code        string     `json:"code,omitempty"`
	Until       *time.Time `json:"until,omitempty"`
	ProbeUntil  *time.Time `json:"probe_until,omitempty"`
	Failures    int        `json:"failures"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// RoutingCircuitsResponse wraps the circuit list so the payload can grow
// without changing its top-level shape.
type RoutingCircuitsResponse struct {
	Circuits []RoutingCircuit `json:"circuits"`
}

// RoutingCircuitReader is the narrow runtime seam behind the read-only
// circuit endpoint. It never opens, closes, or retries a circuit.
type RoutingCircuitReader interface {
	ListRoutingCircuits(ctx context.Context) ([]RoutingCircuit, error)
}

// SetRoutingCircuitReader injects the runtime circuit read seam.
func (c *Controller) SetRoutingCircuitReader(reader RoutingCircuitReader) {
	c.routingCircuitReader = reader
}

// ListRoutingCircuits returns the circuits the router is currently acting on.
// An empty list is a real answer, distinct from an unwired reader.
func (c *Controller) ListRoutingCircuits(ctx context.Context) (*RoutingCircuitsResponse, error) {
	if c.routingCircuitReader == nil {
		return nil, ErrRoutingCircuitsUnavailable
	}
	circuits, err := c.routingCircuitReader.ListRoutingCircuits(ctx)
	if err != nil {
		return nil, err
	}
	if circuits == nil {
		circuits = []RoutingCircuit{}
	}
	return &RoutingCircuitsResponse{Circuits: circuits}, nil
}
