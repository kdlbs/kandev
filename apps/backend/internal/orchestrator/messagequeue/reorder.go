package messagequeue

// validateReorderInput rejects empty or duplicate reorder submissions before
// they reach the repository. Duplicate ids are a client bug but map to the
// same ErrQueueChanged drift signal so direct callers reconcile from the
// authoritative queue (the WS handler reports duplicates as a validation
// error before this check runs).
func validateReorderInput(orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return ErrQueueChanged
	}
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, duplicate := seen[id]; duplicate {
			return ErrQueueChanged
		}
		seen[id] = struct{}{}
	}
	return nil
}

// splitVisibleAndReserved partitions a position-ordered row list into visible
// pending entries and reserved in-flight lifecycle rows. Reordering validates
// against the visible set exactly as GetStatus presents it; reserved rows are
// hidden from clients and never reordered.
func splitVisibleAndReserved(stored []*QueuedMessage) (visible, reserved []*QueuedMessage) {
	visible = make([]*QueuedMessage, 0, len(stored))
	for _, msg := range stored {
		if msg.IsReservedInFlight() {
			reserved = append(reserved, msg)
		} else {
			visible = append(visible, msg)
		}
	}
	return visible, reserved
}

// validateReorderSet checks that orderedIDs is an exact permutation of the
// visible pending entry ids and returns the entries in the submitted order.
// Any drift — a length mismatch, a missing or unknown id, or a duplicate —
// returns ErrQueueChanged so callers reject the reorder atomically.
func validateReorderSet(visible []*QueuedMessage, orderedIDs []string) ([]*QueuedMessage, error) {
	if len(orderedIDs) != len(visible) {
		return nil, ErrQueueChanged
	}
	byID := make(map[string]*QueuedMessage, len(visible))
	for _, msg := range visible {
		byID[msg.ID] = msg
	}
	ordered := make([]*QueuedMessage, 0, len(visible))
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, duplicate := seen[id]; duplicate {
			return nil, ErrQueueChanged
		}
		seen[id] = struct{}{}
		msg, ok := byID[id]
		if !ok {
			return nil, ErrQueueChanged
		}
		ordered = append(ordered, msg)
	}
	return ordered, nil
}
