package models

// ConversationMatch carries a bounded excerpt, never the stored transcript.
type ConversationMatch struct {
	SessionID       string   `json:"session_id"`
	MessageID       string   `json:"message_id"`
	AuthorType      string   `json:"author_type"`
	Excerpt         string   `json:"excerpt"`
	HighlightRanges [][2]int `json:"highlight_ranges"`
}

type ConversationSearchHit struct {
	TaskID string
	Match  ConversationMatch
}

type ConversationSearchPage struct {
	Hits    []ConversationSearchHit
	State   string
	HasMore bool
}

const (
	ConversationSearchReady       = "ready"
	ConversationSearchUnavailable = "unavailable"
	ConversationSearchLimited     = "limited"
	ConversationSearchIndexing    = "indexing"
	ConversationQueryTooBroad     = "query_too_broad"
	ConversationQueryTooShort     = "query_too_short"
	ConversationQueryTooLong      = "query_too_long"
)
