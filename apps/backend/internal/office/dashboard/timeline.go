package dashboard

import "strings"

func buildStatusTimeline(changes []TimelineEvent) []TimelineEventDTO {
	timeline := make([]TimelineEventDTO, len(changes))
	for i, ev := range changes {
		timeline[i] = TimelineEventDTO{
			Type: "status_change",
			From: ev.From,
			To:   ev.To,
			At:   ev.At,
		}
	}
	return timeline
}

// timelineStatusIsInProgress and timelineStatusIsDone classify a timeline
// event's "to" value into the office in_progress/done status buckets. The
// activity log can contain API statuses, persisted states, or raw step names.
func timelineStatusIsInProgress(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case statusInProgressLowercase, "in progress":
		return true
	default:
		return false
	}
}

func timelineStatusIsDone(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case statusDoneLowercase, "completed":
		return true
	default:
		return false
	}
}

// deriveTaskTimestamps derives Started and Completed timestamps from the
// status-change timeline. Callers clear Completed when the task is reopened.
func deriveTaskTimestamps(changes []TimelineEvent) (startedAt, completedAt string) {
	for _, ev := range changes {
		switch {
		case timelineStatusIsInProgress(ev.To):
			if startedAt == "" || ev.At < startedAt {
				startedAt = ev.At
			}
		case timelineStatusIsDone(ev.To):
			if ev.At > completedAt {
				completedAt = ev.At
			}
		}
	}
	return startedAt, completedAt
}
