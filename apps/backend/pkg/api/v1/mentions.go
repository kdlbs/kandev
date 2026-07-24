package v1

const EntityReferenceVersion = 1

// MentionStatus is the safe, provider-neutral state of one mention result group.
type MentionStatus string

const (
	MentionStatusOK               MentionStatus = "ok"
	MentionStatusNotConfigured    MentionStatus = "not_configured"
	MentionStatusUnauthorized     MentionStatus = "unauthorized"
	MentionStatusRateLimited      MentionStatus = "rate_limited"
	MentionStatusTimeout          MentionStatus = "timeout"
	MentionStatusUpstreamError    MentionStatus = "upstream_error"
	MentionStatusUnsupportedScope MentionStatus = "unsupported_scope"
)

// EntityReference is the normalized, versioned reference returned by mention search.
type EntityReference struct {
	Version  int    `json:"version"`
	Ref      string `json:"ref"`
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Key      string `json:"key,omitempty"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Scope    string `json:"scope"`
}

// MentionGroup contains results from one registered provider source and kind.
type MentionGroup struct {
	Source      string            `json:"source"`
	Provider    string            `json:"provider"`
	Kind        string            `json:"kind"`
	DisplayName string            `json:"display_name"`
	KindLabel   string            `json:"kind_label"`
	Status      MentionStatus     `json:"status"`
	Results     []EntityReference `json:"results"`
}

// MentionSearchResponse is the deterministic aggregate search response.
type MentionSearchResponse struct {
	Query  string         `json:"query"`
	Groups []MentionGroup `json:"groups"`
}
