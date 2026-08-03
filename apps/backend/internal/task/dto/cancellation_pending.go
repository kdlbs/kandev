package dto

// EnrichCancellationPending stamps the current runtime cancellation state
// onto a full session DTO. The provider is authoritative for both true and
// false values, so a settled cancellation clears stale client state.
func EnrichCancellationPending(session *TaskSessionDTO, provider CancellationPendingProvider) {
	if session == nil || provider == nil {
		return
	}
	session.CancellationPending, session.CancellationRevision = cancellationPendingSnapshot(
		provider,
		session.ID,
	)
}

// EnrichCancellationPendingSummary is the summary DTO equivalent of
// EnrichCancellationPending.
func EnrichCancellationPendingSummary(session *TaskSessionSummaryDTO, provider CancellationPendingProvider) {
	if session == nil || provider == nil {
		return
	}
	session.CancellationPending, session.CancellationRevision = cancellationPendingSnapshot(
		provider,
		session.ID,
	)
}

func cancellationPendingSnapshot(
	provider CancellationPendingProvider,
	sessionID string,
) (bool, uint64) {
	if snapshotProvider, ok := provider.(CancellationPendingSnapshotProvider); ok {
		return snapshotProvider.CancellationPendingSnapshot(sessionID)
	}
	return provider.CancellationPending(sessionID), 0
}
