package models

import "maps"

// PublicTaskMetadata returns a detached task metadata projection for API
// responses. Deferred-launch attribution and step-handoff carry are
// server-owned and must not expose their internal state to clients.
func PublicTaskMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	public := maps.Clone(metadata)
	delete(public, MetaKeyStepHandoffCarry)
	deferred, ok := public[MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		return public
	}
	publicDeferred := maps.Clone(deferred)
	delete(publicDeferred, DeferredLaunchUserIDKey)
	delete(publicDeferred, DeferredLaunchRecordRecentUseKey)
	public[MetaKeyDeferredLaunch] = publicDeferred
	return public
}
