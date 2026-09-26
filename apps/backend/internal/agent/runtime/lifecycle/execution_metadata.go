package lifecycle

import "reflect"

// AgentExecution.metadata is written and read from unrelated goroutines: the
// launch path caches resolved profile env and MCP artifacts on it, the prompt
// path stores model overrides, and the stop path (RemoveExecution ->
// cleanupPassthroughMCPConfig) deletes the passthrough keys. Those overlap in
// practice — a scheduled stop can land while a relaunch is still writing — and
// an unsynchronized Go map crashes the process with "concurrent map writes"
// rather than corrupting quietly.
//
// The field is therefore unexported and every access goes through the helpers
// below, all of which hold metadataMu. Never hand the live map to a caller or
// store it on another struct: use MetadataSnapshot, which returns a copy.

func (e *AgentExecution) setMetadataValue(key string, value interface{}) {
	if e == nil {
		return
	}
	e.metadataMu.Lock()
	defer e.metadataMu.Unlock()
	if e.metadata == nil {
		e.metadata = make(map[string]interface{})
	}
	e.metadata[key] = value
}

// deleteMetadataValues removes keys from the execution's metadata. Deleting an
// absent key is a no-op, so callers need not check first.
func (e *AgentExecution) deleteMetadataValues(keys ...string) {
	if e == nil {
		return
	}
	e.metadataMu.Lock()
	defer e.metadataMu.Unlock()
	if e.metadata == nil {
		return
	}
	for _, key := range keys {
		delete(e.metadata, key)
	}
}

func (e *AgentExecution) metadataValue(key string) (interface{}, bool) {
	if e == nil {
		return nil, false
	}
	e.metadataMu.RLock()
	defer e.metadataMu.RUnlock()
	value, ok := e.metadata[key]
	return value, ok
}

// metadataString returns the value at key when it is a string, and "" otherwise.
func (e *AgentExecution) metadataString(key string) string {
	value, _ := e.metadataValue(key)
	s, _ := value.(string)
	return s
}

// metadataBool returns the value at key when it is a bool, and false otherwise.
func (e *AgentExecution) metadataBool(key string) bool {
	value, _ := e.metadataValue(key)
	b, _ := value.(bool)
	return b
}

// metadataInt returns the value at key coerced to an int, and 0 otherwise. It
// accepts the numeric shapes metadata arrives in after a JSON round-trip.
func (e *AgentExecution) metadataInt(key string) int {
	value, ok := e.metadataValue(key)
	if !ok {
		return 0
	}
	return metadataInt(map[string]interface{}{key: value}, key)
}

// MetadataSnapshot returns a shallow copy of the execution's metadata. This is
// what every caller that needs the whole map must use — passing the live map to
// an executor backend, an MCP resolver, or a persistence shape leaves it being
// read after this function returns, outside the lock.
func (e *AgentExecution) MetadataSnapshot() map[string]interface{} {
	if e == nil {
		return nil
	}
	e.metadataMu.RLock()
	defer e.metadataMu.RUnlock()
	if len(e.metadata) == 0 {
		return nil
	}
	snapshot := make(map[string]interface{}, len(e.metadata))
	for key, value := range e.metadata {
		snapshot[key] = value
	}
	return snapshot
}

// mergeMetadataWithRollback applies a scoped metadata update and returns a
// compare-and-restore rollback. A key is restored only while it still contains
// the value applied here, so a concurrent writer is never overwritten by a
// failed remote refresh.
func (e *AgentExecution) mergeMetadataWithRollback(metadata map[string]interface{}) func() {
	if e == nil || len(metadata) == 0 {
		return func() {}
	}
	e.metadataMu.Lock()
	if e.metadata == nil {
		e.metadata = make(map[string]interface{}, len(metadata))
	}
	prior := make(map[string]interface{}, len(metadata))
	priorExists := make(map[string]bool, len(metadata))
	applied := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		prior[key], priorExists[key] = e.metadata[key]
		e.metadata[key] = value
		applied[key] = value
	}
	e.metadataMu.Unlock()

	return func() {
		e.metadataMu.Lock()
		defer e.metadataMu.Unlock()
		for key, appliedValue := range applied {
			current, exists := e.metadata[key]
			if !exists || !reflect.DeepEqual(current, appliedValue) {
				continue
			}
			if priorExists[key] {
				e.metadata[key] = prior[key]
			} else {
				delete(e.metadata, key)
			}
		}
	}
}
