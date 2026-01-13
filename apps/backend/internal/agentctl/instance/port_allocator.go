// Package instance provides utilities for managing multi-agent instances,
// including dynamic port allocation for agent services.
package instance

import (
	"fmt"
	"sync"
)

// PortAllocator manages dynamic port allocation for multi-agent instances.
// It tracks which ports are in use and provides thread-safe allocation
// and release of ports within a configured range.
type PortAllocator struct {
	basePort  int
	maxPort   int
	allocated map[int]string // maps port to instance ID
	mu        sync.Mutex
}

// NewPortAllocator creates a new PortAllocator that manages ports
// in the range [basePort, maxPort].
func NewPortAllocator(basePort, maxPort int) *PortAllocator {
	return &PortAllocator{
		basePort:  basePort,
		maxPort:   maxPort,
		allocated: make(map[int]string),
	}
}

// Allocate finds and reserves an available port for the given instance ID.
// It performs a linear search starting from basePort up to maxPort.
// Returns the allocated port number, or an error if no ports are available.
func (p *PortAllocator) Allocate(instanceID string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for port := p.basePort; port <= p.maxPort; port++ {
		if _, exists := p.allocated[port]; !exists {
			p.allocated[port] = instanceID
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range [%d, %d]", p.basePort, p.maxPort)
}

// Release frees a port for reuse. If the port is not currently allocated,
// this operation is a no-op.
func (p *PortAllocator) Release(port int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.allocated, port)
}

// IsAvailable checks if a specific port is available for allocation.
func (p *PortAllocator) IsAvailable(port int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, exists := p.allocated[port]
	return !exists
}

