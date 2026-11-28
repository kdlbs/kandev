package messagequeue

import "encoding/json"

// QueueEntryView adds server-owned presentation fields to a pending entry.
type QueueEntryView struct {
	QueuedMessage
	AdmissionReason string `json:"admission_reason,omitempty"`
}

// ProjectedEntries supplies the same entry shape to queue snapshots and status events.
func (s *QueueStatus) ProjectedEntries() []QueueEntryView {
	if s.Entries == nil {
		return nil
	}
	entries := make([]QueueEntryView, len(s.Entries))
	for i, entry := range s.Entries {
		entries[i] = QueueEntryView{QueuedMessage: entry, AdmissionReason: knownAdmissionReason(entry.Metadata)}
	}
	return entries
}

func (s QueueStatus) MarshalJSON() ([]byte, error) {
	type queueStatus QueueStatus
	return json.Marshal(struct {
		queueStatus
		Entries []QueueEntryView `json:"entries"`
	}{
		queueStatus: queueStatus(s),
		Entries:     s.ProjectedEntries(),
	})
}
