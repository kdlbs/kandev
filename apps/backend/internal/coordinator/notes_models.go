package coordinator

import (
	"fmt"
	"net/http"
	"time"
)

// Note types a manager may choose.
const (
	NoteTypeUser      = "user"
	NoteTypeFeedback  = "feedback"
	NoteTypeProject   = "project"
	NoteTypeReference = "reference"
)

// Note limits.
const (
	maxNoteTextRunes   = 500
	maxNoteCitations   = 10
	maxLiveNotes       = 100
	maxNoteBodyBytes   = 64 << 10
	noteIndexLines     = 38
	maxProposalNoteIDs = 5
	noteTTL            = 30 * 24 * time.Hour
)

// Note reason codes carried by every rejected note mutation.
const (
	NoteInvalidType       = "invalid_type"
	NoteInvalidTextLength = "invalid_text_length"
	NoteInvalidCitation   = "invalid_citation"
	NoteCitationUnres     = "citation_unresolved"
	NoteCredential        = "credential_detected"
	NoteLimitReached      = "note_limit_reached"
	NoteInvalidRequest    = "invalid_request"
	NoteInvalidField      = "invalid_field"
	NoteConflict          = "conflict"
	NoteUnavailable       = "unavailable"
)

// Conflict reasons.
const (
	ConflictVersion = "version"
	ConflictExpired = "expired"
	ConflictDeleted = "deleted"
)

// Provenance kinds of a note version.
const (
	ProvenanceHuman = "human"
	ProvenanceModel = "model"
)

// Validation results stored on a note.
const (
	validationValid       = "valid"
	validationStale       = "stale"
	validationUnavailable = "unavailable"
)

// NoteError is a rejected note mutation. Code is from the closed reason set;
// Reason names a conflict; Field and Index locate the offending input.
type NoteError struct {
	Code   string
	Reason string
	Field  string
	Index  *int
	// TooLarge marks a request body over the size limit (HTTP 413).
	TooLarge bool
}

func (e *NoteError) Error() string {
	return fmt.Sprintf("coordinator note: %s %s %s", e.Code, e.Reason, e.Field)
}

// Status is the HTTP status of the rejection.
func (e *NoteError) Status() int {
	if e.TooLarge {
		return http.StatusRequestEntityTooLarge
	}
	switch e.Code {
	case NoteConflict, NoteLimitReached:
		return http.StatusConflict
	case NoteUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func noteErr(code, field string) *NoteError { return &NoteError{Code: code, Field: field} }

func noteConflict(reason string) *NoteError {
	return &NoteError{Code: NoteConflict, Reason: reason}
}

// NoteProvenance records who authored a note version.
type NoteProvenance struct {
	Kind   string `json:"kind"`
	UserID string `json:"user_id,omitempty"`
	Model  string `json:"model,omitempty"`
}

// NoteVersion is one immutable version of a note.
type NoteVersion struct {
	NoteID       string         `json:"note_id"`
	Version      int            `json:"version"`
	Type         string         `json:"type"`
	Text         string         `json:"text"`
	Citations    []NoteCitation `json:"citations"`
	Provenance   NoteProvenance `json:"provenance"`
	ActorID      string         `json:"actor_id,omitempty"`
	RevertedFrom int            `json:"reverted_from,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// Note is a coordinator note row with its current version's content.
type Note struct {
	ID            string
	CoordinatorID string
	WorkspaceID   string
	SourceItemID  string
	CurrentVer    int
	CreatedAt     time.Time
	ExpiresAt     time.Time
	DeletedAt     *time.Time
	UseCount      int
	// LastValidatedAt and LastResult are the latest successful validation of
	// the current version; LastAttemptAt and LastAttempt are the latest
	// attempt of any outcome.
	LastValidatedAt *time.Time
	LastResult      string
	LastAttemptAt   *time.Time
	LastAttempt     string
	Current         NoteVersion
}

func (n *Note) expired(at time.Time) bool { return !n.ExpiresAt.After(at) }

// NoteStatus is the single state settings display for a note.
type NoteStatus string

// The display states, in precedence order.
const (
	NoteStatusDeleted      NoteStatus = "deleted"
	NoteStatusExpired      NoteStatus = "expired"
	NoteStatusStale        NoteStatus = "stale"
	NoteStatusUnavailable  NoteStatus = "unavailable"
	NoteStatusNotValidated NoteStatus = "not_validated"
	NoteStatusOverCap      NoteStatus = "over_cap"
	NoteStatusLive         NoteStatus = "live"
)

// baseStatus is the status before the over-the-cap derivation.
func (n *Note) baseStatus(at time.Time) NoteStatus {
	switch {
	case n.DeletedAt != nil:
		return NoteStatusDeleted
	case n.expired(at):
		return NoteStatusExpired
	case n.LastResult == validationStale:
		return NoteStatusStale
	case n.LastAttempt == validationUnavailable:
		return NoteStatusUnavailable
	case n.LastResult == "":
		return NoteStatusNotValidated
	}
	return NoteStatusLive
}
