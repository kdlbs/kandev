package models

import "maps"

// PublicTaskMetadata returns a detached task metadata projection for API
// responses. Deferred-launch attribution and step-handoff carry are
// server-owned and must not expose their internal state to clients. The
// transient workflow-move marker carries encoded one-shot instructions and an
// internal move ID; it is server-owned lifecycle state and must never reach a
// public event or DTO.
//
// The session-ceiling replay payload is redacted for a stronger reason than
// tidiness: it carries the launch-scoped environment map, the composed prompt, its
// attachments and entity references. The record's discriminators, timestamps and
// reason code stay projected, so a client can still see that a deferral is pending,
// of which kind, since when and why.
func PublicTaskMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	public := maps.Clone(metadata)
	delete(public, MetaKeyWorkflowMovePending)
	delete(public, MetaKeyStepHandoffCarry)
	deferred, ok := public[MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		return public
	}
	publicDeferred := maps.Clone(deferred)
	delete(publicDeferred, DeferredLaunchUserIDKey)
	delete(publicDeferred, DeferredLaunchRecordRecentUseKey)
	delete(publicDeferred, CeilingLaunchPayloadKey)
	delete(publicDeferred, CeilingLaunchClaimKey)
	public[MetaKeyDeferredLaunch] = publicDeferred
	return public
}
