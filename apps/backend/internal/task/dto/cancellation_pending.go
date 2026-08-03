package dto

// EnrichCancellationPending stamps the current runtime cancellation state
// onto a full session DTO. The provider is authoritative for both true and
// false values, so a settled cancellation clears stale client state.
func EnrichCancellationPending(session *TaskSessionDTO, provider CancellationPendingProvider) {
	if session == nil || provider == nil {
		return
	}
	session.CancellationPending = provider.CancellationPending(session.ID)
}

// EnrichCancellationPendingSummary is the summary DTO equivalent of
// EnrichCancellationPending.
func EnrichCancellationPendingSummary(session *TaskSessionSummaryDTO, provider CancellationPendingProvider) {
	if session == nil || provider == nil {
		return
	}
	session.CancellationPending = provider.CancellationPending(session.ID)
}
