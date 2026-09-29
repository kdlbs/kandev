package service

const (
	// DefaultMessagesPageSize is the bounded first-page size returned when a
	// history request supplies no pagination parameters at all (see
	// httpListMessages/wsListMessages), matching the PR-watch/storage-bounds
	// plan's requirement that normal history hydration never eagerly
	// materializes a whole session's messages by default.
	DefaultMessagesPageSize = 50
	MaxMessagesPageSize     = 100
)
