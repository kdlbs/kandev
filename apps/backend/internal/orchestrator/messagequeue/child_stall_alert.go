package messagequeue

func buildChildStallMergedEntry(target, source *QueuedMessage) (*autoMergedValues, bool) {
	if !childStallAlertsMergeable(target, source) || target.IsDurableDelivery() || source.IsDurableDelivery() ||
		target.IsReservedInFlight() || source.IsReservedInFlight() || target.IsDeliveryAttempted() || source.IsDeliveryAttempted() || len(source.Attachments) != 0 {
		return nil, false
	}
	return &autoMergedValues{
		content: joinAutoMergeContent(target.Content, source.Content), attachments: append([]MessageAttachment(nil), target.Attachments...),
		metadata: mergeChildStallAlertMetadata(target.Metadata, source.Metadata), queuedAt: latestQueuedAt(target.QueuedAt, source.QueuedAt),
	}, true
}

// MetadataChildStallAlert marks a server-queued child-turn stalled alert.
const MetadataChildStallAlert = "child_stall_alert"

// MetadataChildStallAlerts lists the alert descriptors carried by one queued
// item. Folding unions the lists of the two items.
const MetadataChildStallAlerts = "child_stall_alerts"

// MetadataChildStallNudge marks a server-queued reminder sent to the stalled
// child's own session. It is not a parent alert, so parent-alert folding and
// pruning never treat it as one.
const MetadataChildStallNudge = "child_stall_nudge"

// IsChildStallAlert reports whether a queue entry is a child-turn stalled alert.
func IsChildStallAlert(message *QueuedMessage) bool {
	if message == nil || message.QueuedBy != QueuedByServer || message.Metadata == nil {
		return false
	}
	alert, _ := message.Metadata[MetadataChildStallAlert].(bool)
	return alert
}

// IsChildStallNudge reports whether a queue entry is a reminder queued for a
// stalled child's own session.
func IsChildStallNudge(message *QueuedMessage) bool {
	if message == nil || message.QueuedBy != QueuedByServer || message.Metadata == nil {
		return false
	}
	nudge, _ := message.Metadata[MetadataChildStallNudge].(bool)
	return nudge
}

// admissionFoldEnabled reports whether admission may fold candidate into the
// entry above it. Child-turn stalled alerts always fold into an adjacent
// pending alert for the same task so a busy parent handles them in one turn;
// every other entry follows the session's Auto-merge policy.
func admissionFoldEnabled(policy *AutoMergePolicy, candidate *QueuedMessage) bool {
	return (policy != nil && policy.Enabled) || IsChildStallAlert(candidate)
}

func childStallAlertsMergeable(target, source *QueuedMessage) bool {
	return IsChildStallAlert(target) && IsChildStallAlert(source) &&
		target.TaskID == source.TaskID && target.Model == source.Model && target.PlanMode == source.PlanMode
}

// mergeChildStallAlertMetadata keeps the target's metadata, unions the alert
// lists, and drops single-sender attribution when the folded alerts come
// from different children.
func mergeChildStallAlertMetadata(target, source map[string]interface{}) map[string]interface{} {
	merged := copyMessageMetadata(target, 0)
	alerts := append(childStallAlertList(target), childStallAlertList(source)...)
	merged[MetadataChildStallAlerts] = alerts
	if metadataString(target, MetadataSenderTaskID) != metadataString(source, MetadataSenderTaskID) {
		delete(merged, MetadataSenderTaskID)
		delete(merged, "sender_task_title")
		delete(merged, "sender_session_id")
	}
	return merged
}

func childStallAlertList(metadata map[string]interface{}) []interface{} {
	switch value := metadata[MetadataChildStallAlerts].(type) {
	case []interface{}:
		return append([]interface{}(nil), value...)
	case []map[string]interface{}:
		list := make([]interface{}, 0, len(value))
		for _, item := range value {
			list = append(list, item)
		}
		return list
	default:
		return nil
	}
}

// ChildStallAlertDescriptor is one child-turn stalled candidate folded into a
// queued alert entry.
type ChildStallAlertDescriptor struct {
	ChildTaskID string
	TurnID      string
	Cause       string
}

// ChildStallAlertDescriptors returns the candidate descriptors folded into a
// child-turn stalled alert entry, in stored order. A non-alert entry yields nil.
func ChildStallAlertDescriptors(message *QueuedMessage) []ChildStallAlertDescriptor {
	if !IsChildStallAlert(message) {
		return nil
	}
	raw := childStallAlertList(message.Metadata)
	descriptors := make([]ChildStallAlertDescriptor, 0, len(raw))
	for _, item := range raw {
		descriptor, _ := item.(map[string]interface{})
		childID, _ := descriptor["child_task_id"].(string)
		turnID, _ := descriptor["turn_id"].(string)
		cause, _ := descriptor["cause"].(string)
		descriptors = append(descriptors, ChildStallAlertDescriptor{
			ChildTaskID: childID,
			TurnID:      turnID,
			Cause:       cause,
		})
	}
	return descriptors
}

// ChildStallAlertChildIDs returns the distinct child task IDs a child-turn
// stalled alert entry refers to, in first-seen order.
func ChildStallAlertChildIDs(message *QueuedMessage) []string {
	descriptors := ChildStallAlertDescriptors(message)
	if len(descriptors) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(descriptors))
	ids := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if descriptor.ChildTaskID == "" {
			continue
		}
		if _, ok := seen[descriptor.ChildTaskID]; ok {
			continue
		}
		seen[descriptor.ChildTaskID] = struct{}{}
		ids = append(ids, descriptor.ChildTaskID)
	}
	return ids
}
