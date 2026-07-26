package controller

import (
	"fmt"
	"sync"
)

type MaintenanceKind string

const (
	MaintenanceKindInstall MaintenanceKind = "install"
	MaintenanceKindUpdate  MaintenanceKind = "update"
)

type MaintenanceJobRef struct {
	JobID string          `json:"job_id"`
	Kind  MaintenanceKind `json:"kind"`
}

// MaintenanceConflictError identifies the active opposite-kind operation.
type MaintenanceConflictError struct {
	AgentName string
	Active    MaintenanceJobRef
}

func (e *MaintenanceConflictError) Error() string {
	return fmt.Sprintf("%s already has an active %s job", e.AgentName, e.Active.Kind)
}

// maintenanceCoordinator serializes mutating maintenance per agent while
// allowing independent agents to update or install concurrently.
type maintenanceCoordinator struct {
	mu      sync.Mutex
	byAgent map[string]MaintenanceJobRef
}

func newMaintenanceCoordinator() *maintenanceCoordinator {
	return &maintenanceCoordinator{byAgent: make(map[string]MaintenanceJobRef)}
}

func (c *maintenanceCoordinator) claim(
	agentName string,
	kind MaintenanceKind,
	jobID string,
) (MaintenanceJobRef, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if active, ok := c.byAgent[agentName]; ok {
		if active.Kind == kind {
			return active, false, nil
		}
		return active, false, &MaintenanceConflictError{AgentName: agentName, Active: active}
	}
	ref := MaintenanceJobRef{JobID: jobID, Kind: kind}
	c.byAgent[agentName] = ref
	return ref, true, nil
}

func (c *maintenanceCoordinator) release(agentName string, ref MaintenanceJobRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current, ok := c.byAgent[agentName]; ok && current == ref {
		delete(c.byAgent, agentName)
	}
}
