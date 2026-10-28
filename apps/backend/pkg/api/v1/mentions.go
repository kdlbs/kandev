package v1

const EntityReferenceVersion = 1

type EntityReferenceAssociationMode string

const (
	EntityReferenceModeContextOnly EntityReferenceAssociationMode = "context_only"
	EntityReferenceModeLinkOnSend  EntityReferenceAssociationMode = "link_on_send"
)

type EntityReferenceRegistrationStatus string

const (
	EntityReferenceRegistrationPending    EntityReferenceRegistrationStatus = "pending"
	EntityReferenceRegistrationLinked     EntityReferenceRegistrationStatus = "linked"
	EntityReferenceRegistrationFailed     EntityReferenceRegistrationStatus = "failed"
	EntityReferenceRegistrationSuppressed EntityReferenceRegistrationStatus = "suppressed"
	EntityReferenceRegistrationUncertain  EntityReferenceRegistrationStatus = "uncertain"
	EntityReferenceRegistrationUnlinked   EntityReferenceRegistrationStatus = "unlinked"
)

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
	Version               int                               `json:"version"`
	Ref                   string                            `json:"ref"`
	Provider              string                            `json:"provider"`
	Kind                  string                            `json:"kind"`
	ID                    string                            `json:"id"`
	Key                   string                            `json:"key,omitempty"`
	Title                 string                            `json:"title"`
	URL                   string                            `json:"url"`
	Scope                 string                            `json:"scope"`
	SelectionID           string                            `json:"selection_id,omitempty"`
	AssociationMode       EntityReferenceAssociationMode    `json:"association_mode,omitempty"`
	RegistrationStatus    EntityReferenceRegistrationStatus `json:"registration_status,omitempty"`
	RegistrationErrorCode string                            `json:"registration_error_code,omitempty"`
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
