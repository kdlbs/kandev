package models

import "maps"

// PublicTaskMetadata returns a detached task metadata projection for API
// responses. Deferred launch attribution is server-owned and must never expose
// the creator's raw user ID or the internal recency marker.
func PublicTaskMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	public := maps.Clone(metadata)
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
