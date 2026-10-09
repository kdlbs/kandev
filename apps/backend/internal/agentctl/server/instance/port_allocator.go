// Package instance provides utilities for managing multi-agent instances,
// including dynamic port allocation for agent services.
package instance

import (
	"fmt"
	"sync"
)

// PortLease identifies one allocation for one owner. Generation changes every
// time an allocation is created, including when the same owner reuses a port.
type PortLease struct {
	Port       int
	Owner      string
	Generation uint64
}

// PortAllocator manages dynamic port allocation for multi-agent instances.
// It tracks which ports are in use and provides thread-safe allocation
// and release of ports within a configured range.
type PortAllocator struct {
	basePort    int
	maxPort     int
	allocated   map[int]PortLease
	owners      map[string]PortLease
	unavailable map[int]struct{}
	nextGen     uint64
	mu          sync.Mutex
}

// NewPortAllocator creates a new PortAllocator that manages ports
// in the range [basePort, maxPort].
func NewPortAllocator(basePort, maxPort int) *PortAllocator {
	return &PortAllocator{
		basePort:    basePort,
		maxPort:     maxPort,
		allocated:   make(map[int]PortLease),
		owners:      make(map[string]PortLease),
		unavailable: make(map[int]struct{}),
	}
}

// Allocate finds and reserves an available port for the given instance ID.
// It performs a linear search starting from basePort up to maxPort.
// Existing owners receive their current reservation. A new lease is assigned
// a nonzero, process-local generation.
func (p *PortAllocator) Allocate(owner string) (PortLease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if lease, exists := p.owners[owner]; exists {
		return lease, nil
	}

	for port := p.basePort; port <= p.maxPort; port++ {
		if _, blocked := p.unavailable[port]; blocked {
			continue
		}
		if _, exists := p.allocated[port]; !exists {
			if p.nextGen == ^uint64(0) {
				return PortLease{}, fmt.Errorf("port lease generation exhausted")
			}
			p.nextGen++
			lease := PortLease{Port: port, Owner: owner, Generation: p.nextGen}
			p.allocated[port] = lease
			p.owners[owner] = lease
			return lease, nil
		}
	}

	return PortLease{}, fmt.Errorf("no available ports in range [%d, %d]", p.basePort, p.maxPort)
}

// Release frees a reservation only when the complete lease still matches.
func (p *PortAllocator) Release(lease PortLease) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if current, exists := p.allocated[lease.Port]; !exists || current != lease {
		return
	}
	delete(p.allocated, lease.Port)
	delete(p.owners, lease.Owner)
}

// MarkUnavailable blocks a port only when the complete lease still matches.
func (p *PortAllocator) MarkUnavailable(lease PortLease) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if current, exists := p.allocated[lease.Port]; !exists || current != lease {
		return
	}
	p.unavailable[lease.Port] = struct{}{}
	delete(p.allocated, lease.Port)
	delete(p.owners, lease.Owner)
}
