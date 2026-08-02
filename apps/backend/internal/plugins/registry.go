package plugins

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/plugins/store"
)

// Registry is an in-memory, mutex-guarded index of installed plugins, keyed
// by plugin id. It is loaded from the filesystem store at startup (Load)
// and kept in sync as Service mutates installations, so read paths (List,
// Get) never hit disk.
//
// Get and List return copies of the stored *store.Record so callers cannot
// mutate registry state by holding onto a returned pointer; all writes go
// through Add / Remove / SetStatus / SetRuntimeState / SetAutoUpdate /
// SetRestartCount.
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
		byID[rec.ID] = cloneRecord(rec)
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
	return cloneRecord(rec), true
}

// List returns a copy of every registered record, sorted by id for
// deterministic output.
func (r *Registry) List() []*store.Record {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*store.Record, 0, len(r.byID))
	for _, rec := range r.byID {
		out = append(out, cloneRecord(rec))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Add inserts or replaces the record for record.ID.
func (r *Registry) Add(record *store.Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[record.ID] = cloneRecord(record)
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
	return cloneRecord(rec), true
}

// SetAutoUpdate updates the in-memory per-plugin auto-update override for id
// and returns a copy of the updated record. ok is false if id is not
// registered. Called by Service.SetPluginAutoUpdate, which persists the copy
// to the store; v is nil to clear the override (inherit the global default).
func (r *Registry) SetAutoUpdate(id string, v *bool) (*store.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	rec.AutoUpdate = v
	return cloneRecord(rec), true
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
	return cloneRecord(rec), true
}

// SetRuntimeState updates the lifecycle status and persisted failure
// diagnostic for id and returns a copy of the resulting record. The caller is
// responsible for FSM validation and persistence; this method only performs
// the in-memory mutation under the registry lock.
func (r *Registry) SetRuntimeState(id string, status Status, lastError string, lastErrorAt *time.Time) (*store.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	rec.Status = status
	rec.LastError = lastError
	if lastErrorAt == nil {
		rec.LastErrorAt = nil
	} else {
		at := *lastErrorAt
		rec.LastErrorAt = &at
	}
	return cloneRecord(rec), true
}

func cloneRecord(rec *store.Record) *store.Record {
	clone := *rec
	if rec.AutoUpdate != nil {
		autoUpdate := *rec.AutoUpdate
		clone.AutoUpdate = &autoUpdate
	}
	if rec.LastErrorAt != nil {
		at := *rec.LastErrorAt
		clone.LastErrorAt = &at
	}
	return &clone
}
