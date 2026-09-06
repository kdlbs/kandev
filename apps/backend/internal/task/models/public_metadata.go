package models

import "maps"

// PublicTaskMetadata returns a detached task metadata projection for API
// responses. Deferred launch attribution is server-owned and must never expose
// the creator's raw user ID or the internal recency marker. The transient
// workflow-move marker carries encoded one-shot instructions and an internal
// move ID; it is server-owned lifecycle state and must never reach a public
// event or DTO.
func PublicTaskMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	public := maps.Clone(metadata)
	delete(public, MetaKeyWorkflowMovePending)
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
