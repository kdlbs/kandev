package plugins

import (
	"fmt"
	"sort"
	"sync"

	"github.com/kandev/kandev/internal/plugins/store"
)

// Registry is an in-memory, mutex-guarded index of installed plugins, keyed
// by plugin id. It is loaded from the filesystem store at startup (Load)
// and kept in sync as Service mutates installations, so read paths (List,
// Get) never hit disk.
//
// Get and List return copies of the stored *store.Record so callers cannot
// mutate registry state by holding onto a returned pointer; all writes go
// through Add / Remove / SetStatus / SetRestartCount.
type Registry struct {
	mu   sync.RWMutex
	byID map[string]*store.Record
}

// NewRegistry returns an empty Registry. Call Load to populate it from a
// store.Store at startup.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]*store.Record)}
}

// Load replaces the registry's contents with every record currently
// persisted in s. Intended to be called once at startup (see Provide).
func (r *Registry) Load(s store.Store) error {
	records, err := s.List()
	if err != nil {
		return fmt.Errorf("load plugin registry: %w", err)
	}

	byID := make(map[string]*store.Record, len(records))
	for _, rec := range records {
		byID[rec.ID] = rec
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = byID
	return nil
}

// Get returns a copy of the record for id, and whether it was found.
func (r *Registry) Get(id string) (*store.Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	clone := *rec
	return &clone, true
}

// List returns a copy of every registered record, sorted by id for
// deterministic output.
func (r *Registry) List() []*store.Record {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*store.Record, 0, len(r.byID))
	for _, rec := range r.byID {
		clone := *rec
		out = append(out, &clone)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Add inserts or replaces the record for record.ID.
func (r *Registry) Add(record *store.Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *record
	r.byID[record.ID] = &clone
}

// Remove deletes the record for id, if present. A no-op if id is unknown.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}

// SetStatus updates the in-memory status for id and returns a copy of the
// updated record. ok is false if id is not registered. Callers wanting FSM
// validation (Service.SetStatus) must check that before calling this — this
// method performs the raw mutation only.
func (r *Registry) SetStatus(id string, status Status) (*store.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	rec.Status = status
	clone := *rec
	return &clone, true
}

// SetRestartCount updates the in-memory restart count for id and returns a
// copy of the updated record. ok is false if id is not registered. Called
// by Service after a runtime.Manager-driven restart, to persist
// store.Record.RestartCount best-effort.
func (r *Registry) SetRestartCount(id string, count int) (*store.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	rec.RestartCount = count
	clone := *rec
	return &clone, true
}
