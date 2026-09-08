package requiredstores

import (
	"fmt"
	"sync"
	"time"
)

// State is the initialization or probe state of a required store.
type State string

const (
	StateInitializing State = "initializing"
	StateHealthy      State = "healthy"
	StateUnhealthy    State = "unhealthy"
)

// Status is a point-in-time copy of a catalog entry and its lifecycle state.
type Status struct {
	ID             string
	OwnerPackage   string
	RequiredTables []string
	Capabilities   []Capability
	State          State
	Error          string
	InitializedAt  time.Time
	LastCheckedAt  time.Time
}

// Tracker records required-store initialization while enforcing dependency
// order. Independent stores may be initialized in different bootstrap phases,
// but every declared dependency must be recorded first.
type Tracker struct {
	mu       sync.RWMutex
	catalog  []Descriptor
	byID     map[string]int
	statuses []Status
	recorded []bool
}

// NewTracker validates and copies a catalog before tracking it.
func NewTracker(descriptors []Descriptor) (*Tracker, error) {
	if err := ValidateCatalog(descriptors); err != nil {
		return nil, fmt.Errorf("validate required-store catalog: %w", err)
	}
	cloned := cloneDescriptors(descriptors)
	byID := make(map[string]int, len(cloned))
	statuses := make([]Status, len(cloned))
	for index, descriptor := range cloned {
		byID[descriptor.ID] = index
		statuses[index] = statusFor(descriptor, StateInitializing)
	}
	return &Tracker{catalog: cloned, byID: byID, statuses: statuses, recorded: make([]bool, len(cloned))}, nil
}

// NewCatalogTracker creates a tracker for the built-in catalog.
func NewCatalogTracker() (*Tracker, error) {
	return NewTracker(Catalog())
}

// Record records one store initialization result. Results must be reported
// after all declared dependencies, which makes omissions and accidental phase
// reordering visible before readiness.
func (t *Tracker) Record(id string, initErr error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	index, exists := t.byID[id]
	if !exists {
		return fmt.Errorf("unknown required store %q", id)
	}
	if t.recorded[index] {
		return fmt.Errorf("duplicate required store result for %q", id)
	}
	for _, dependency := range t.catalog[index].DependsOn {
		dependencyIndex := t.byID[dependency]
		if !t.recorded[dependencyIndex] {
			return fmt.Errorf("required store %q is out of order, waiting for %q", id, dependency)
		}
	}
	state := StateHealthy
	message := ""
	if initErr != nil {
		state = StateUnhealthy
		message = initErr.Error()
	}
	now := time.Now().UTC()
	t.statuses[index].State = state
	t.statuses[index].Error = message
	t.statuses[index].InitializedAt = now
	t.statuses[index].LastCheckedAt = now
	t.recorded[index] = true
	return nil
}

// RecordSuccess is a convenience wrapper for successful initialization.
func (t *Tracker) RecordSuccess(id string) error {
	return t.Record(id, nil)
}

// RecordFailure is a convenience wrapper for failed initialization.
func (t *Tracker) RecordFailure(id string, initErr error) error {
	return t.Record(id, initErr)
}

// RecordProbe updates the runtime result for an already initialized store.
// Probe results may transition between healthy and unhealthy for the lifetime
// of the process, unlike initialization results which are recorded once.
func (t *Tracker) RecordProbe(id string, probeErr error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	index, exists := t.byID[id]
	if !exists {
		return fmt.Errorf("unknown required store %q", id)
	}
	if !t.recorded[index] {
		return fmt.Errorf("required store %q has not been initialized", id)
	}
	status := &t.statuses[index]
	status.State = StateHealthy
	status.Error = ""
	if probeErr != nil {
		status.State = StateUnhealthy
		status.Error = probeErr.Error()
	}
	status.LastCheckedAt = time.Now().UTC()
	return nil
}

// AggregateState returns the worst state currently observed across the
// catalog. Initialization is incomplete until every descriptor is recorded.
func (t *Tracker) AggregateState() State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for index, status := range t.statuses {
		if !t.recorded[index] || status.State == StateInitializing {
			return StateInitializing
		}
		if status.State == StateUnhealthy {
			return StateUnhealthy
		}
	}
	return StateHealthy
}

// Healthy reports whether initialization is complete and every runtime probe
// currently succeeds.
func (t *Tracker) Healthy() bool {
	return t.AggregateState() == StateHealthy
}

// UnhealthyStoreIDs returns unhealthy stores in catalog order.
func (t *Tracker) UnhealthyStoreIDs() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := make([]string, 0)
	for _, status := range t.statuses {
		if status.State == StateUnhealthy {
			ids = append(ids, status.ID)
		}
	}
	return ids
}

// UnavailableStoreIDs returns every store that is not currently healthy,
// including stores that have not completed initialization yet.
func (t *Tracker) UnavailableStoreIDs() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := make([]string, 0)
	for index, status := range t.statuses {
		if !t.recorded[index] || status.State != StateHealthy {
			ids = append(ids, status.ID)
		}
	}
	return ids
}

// ValidateComplete verifies that every catalog entry was reported and that no
// required store reported an initialization failure.
func (t *Tracker) ValidateComplete() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	missing := 0
	firstMissing := ""
	for index, recorded := range t.recorded {
		if recorded {
			continue
		}
		missing++
		if firstMissing == "" {
			firstMissing = t.catalog[index].ID
		}
	}
	if missing > 0 {
		return fmt.Errorf("required-store catalog is missing %d result(s), next %q", missing, firstMissing)
	}
	for _, status := range t.statuses {
		if status.State == StateUnhealthy {
			return fmt.Errorf("required store %q is unhealthy: %s", status.ID, status.Error)
		}
	}
	return nil
}

// Complete is an alias for ValidateComplete.
func (t *Tracker) Complete() error {
	return t.ValidateComplete()
}

// NextID returns the next catalog ID that must be reported, or an empty string
// after all entries have been recorded.
func (t *Tracker) NextID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for index, recorded := range t.recorded {
		if !recorded {
			return t.catalog[index].ID
		}
	}
	return ""
}

// Snapshot returns statuses in catalog order. Returned slices are independent
// copies and can be safely inspected by diagnostics or health probes.
func (t *Tracker) Snapshot() []Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]Status, len(t.statuses))
	for index, status := range t.statuses {
		result[index] = status
		result[index].RequiredTables = append([]string(nil), status.RequiredTables...)
		result[index].Capabilities = append([]Capability(nil), status.Capabilities...)
	}
	return result
}

// Status returns a status copy for one catalog ID.
func (t *Tracker) Status(id string) (Status, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	index, ok := t.byID[id]
	if !ok {
		return Status{}, false
	}
	status := t.statuses[index]
	status.RequiredTables = append([]string(nil), status.RequiredTables...)
	status.Capabilities = append([]Capability(nil), status.Capabilities...)
	return status, true
}

func statusFor(descriptor Descriptor, state State) Status {
	return Status{
		ID:             descriptor.ID,
		OwnerPackage:   descriptor.OwnerPackage,
		RequiredTables: append([]string(nil), descriptor.RequiredTables...),
		Capabilities:   append([]Capability(nil), descriptor.Capabilities...),
		State:          state,
	}
}
