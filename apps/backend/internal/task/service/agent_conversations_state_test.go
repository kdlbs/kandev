package service

import (
	"context"
	"encoding/json"
	"sync"
)

// acFakeStateRepo simulates the durable atomic Claim primitive
// (internal/plugins/state.Store.Claim in production) with an in-process
// mutex-guarded map. It proves service behavior while Store.Claim's SQL-level
// atomicity and restart durability are proven separately in internal/plugins/state.
type acFakeStateRepo struct {
	mu     sync.Mutex
	claims map[string]bool
	values map[string]json.RawMessage
}

func newACFakeStateRepo() *acFakeStateRepo {
	return &acFakeStateRepo{claims: map[string]bool{}, values: map[string]json.RawMessage{}}
}

func acStateKey(pluginID, scope, scopeID, key string) string {
	return pluginID + "/" + scope + "/" + scopeID + "/" + key
}

func (f *acFakeStateRepo) Claim(_ context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := acStateKey(pluginID, scope, scopeID, key)
	if f.claims[k] {
		return false, nil
	}
	f.claims[k] = true
	f.values[k] = append(json.RawMessage(nil), value...)
	return true, nil
}

func (f *acFakeStateRepo) Get(_ context.Context, pluginID, scope, scopeID, key string) (json.RawMessage, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, found := f.values[acStateKey(pluginID, scope, scopeID, key)]
	if !found {
		return nil, false, nil
	}
	return append(json.RawMessage(nil), value...), true, nil
}

func (f *acFakeStateRepo) Set(_ context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := acStateKey(pluginID, scope, scopeID, key)
	f.claims[k] = true
	f.values[k] = append(json.RawMessage(nil), value...)
	return nil
}

func (f *acFakeStateRepo) Delete(_ context.Context, pluginID, scope, scopeID, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := acStateKey(pluginID, scope, scopeID, key)
	delete(f.claims, k)
	delete(f.values, k)
	return nil
}

func (f *acFakeStateRepo) seedPending(pluginID, workspaceID, conversationKey, occurrenceKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := acStateKey(pluginID, agentConversationOccurrenceScope, workspaceID+"/"+conversationKey, occurrenceKey)
	f.claims[k] = true
	f.values[k], _ = json.Marshal(map[string]interface{}{
		"status":           agentConversationOccurrencePending,
		"plugin_id":        pluginID,
		"workspace_id":     workspaceID,
		"conversation_key": conversationKey,
	})
}
