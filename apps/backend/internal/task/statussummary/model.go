// Package statussummary contains the bounded, task-level status projection
// used by task lists and switchers. It deliberately contains no transcript,
// Git file, or provider payload data.
package statussummary

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"
)

const (
	// MaxActiveErrorPreviewBytes keeps an error decoration safe to send with
	// every task row without turning it into a message-stream transport.
	MaxActiveErrorPreviewBytes = 512
	maxSessionIDBytes          = 256
	maxPendingActionBytes      = 128
	maxActiveErrorStampBytes   = 64
	maxPullRequestStateBytes   = 64
	maxPullRequestURLBytes     = 2048
)

// TaskStatusSummary is the complete replacement value delivered to task-list
// consumers. Revision and UpdatedAt are transport metadata and are ignored by
// SemanticEqual when deciding whether a projection actually changed.
type TaskStatusSummary struct {
	Revision            uint64                 `json:"revision"`
	UpdatedAt           time.Time              `json:"updated_at"`
	PrimarySession      *PrimarySessionSummary `json:"primary_session,omitempty"`
	ForegroundActivity  string                 `json:"foreground_activity,omitempty"`
	ActiveSubagentCount int                    `json:"active_subagent_count,omitempty"`
	PendingAction       string                 `json:"pending_action,omitempty"`
	ActiveError         *ActiveErrorSummary    `json:"active_error,omitempty"`
	Git                 *GitSummary            `json:"git,omitempty"`
	PullRequest         *PullRequestSummary    `json:"pull_request,omitempty"`
}

type PrimarySessionSummary struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

type ActiveErrorSummary struct {
	SessionID  string    `json:"session_id"`
	Stamp      string    `json:"stamp"`
	OccurredAt time.Time `json:"occurred_at"`
	Preview    string    `json:"preview"`
}

type GitSummary struct {
	Additions    int `json:"additions,omitempty"`
	Deletions    int `json:"deletions,omitempty"`
	ChangedFiles int `json:"changed_files,omitempty"`
	Ahead        int `json:"ahead,omitempty"`
	Behind       int `json:"behind,omitempty"`
}

// PullRequestSummary is intentionally an aggregate plus one representative
// identity. It is not a list of PR records.
type PullRequestSummary struct {
	Count          int    `json:"count,omitempty"`
	OpenCount      int    `json:"open_count,omitempty"`
	Attention      bool   `json:"attention,omitempty"`
	AggregateState string `json:"aggregate_state,omitempty"`
	State          string `json:"state,omitempty"`
	Number         int    `json:"number,omitempty"`
	URL            string `json:"url,omitempty"`
}

// StoredTaskStatusSummary is the persistence boundary for one task. The
// workspace ID is kept beside the payload so future event routing can verify
// ownership without decoding arbitrary summary data.
type StoredTaskStatusSummary struct {
	TaskID      string
	WorkspaceID string
	Summary     TaskStatusSummary
}

// SemanticEqual compares only fields that task consumers observe as status.
func (s TaskStatusSummary) SemanticEqual(other TaskStatusSummary) bool {
	s.Revision = 0
	s.UpdatedAt = time.Time{}
	other.Revision = 0
	other.UpdatedAt = time.Time{}
	return reflect.DeepEqual(s, other)
}

// Validate enforces the bounded fields at the persistence boundary. Other
// source-specific limits belong to the projector that derives this model.
func (s TaskStatusSummary) Validate() error {
	if s.ActiveSubagentCount < 0 {
		return fmt.Errorf("active subagent count cannot be negative")
	}
	if err := validatePrimarySession(s.PrimarySession); err != nil {
		return err
	}
	if err := validateUTF8Bytes("pending action", s.PendingAction, maxPendingActionBytes); err != nil {
		return err
	}
	if err := validateActiveError(s.ActiveError); err != nil {
		return err
	}
	return validatePullRequest(s.PullRequest)
}

func validatePrimarySession(session *PrimarySessionSummary) error {
	if session == nil {
		return nil
	}
	if err := validateUTF8Bytes("primary session id", session.ID, maxSessionIDBytes); err != nil {
		return err
	}
	return validateUTF8Bytes("primary session state", session.State, maxPendingActionBytes)
}

func validateActiveError(activeError *ActiveErrorSummary) error {
	if activeError == nil {
		return nil
	}
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{"active error session id", activeError.SessionID, maxSessionIDBytes},
		{"active error stamp", activeError.Stamp, maxSessionIDBytes},
		{"active error preview", activeError.Preview, MaxActiveErrorPreviewBytes},
	}
	for _, field := range fields {
		if err := validateUTF8Bytes(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	return nil
}

func validatePullRequest(pr *PullRequestSummary) error {
	if pr == nil {
		return nil
	}
	if pr.Count < 0 || pr.OpenCount < 0 || pr.Number < 0 {
		return fmt.Errorf("pull request counts and number cannot be negative")
	}
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{"pull request state", pr.State, maxPullRequestStateBytes},
		{"pull request aggregate state", pr.AggregateState, maxPullRequestStateBytes},
		{"pull request URL", pr.URL, maxPullRequestURLBytes},
	}
	for _, field := range fields {
		if err := validateUTF8Bytes(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	return nil
}

func validateUTF8Bytes(field, value string, maxBytes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return nil
}

// SemanticJSON returns the canonical payload stored in SQLite/Postgres. The
// revision and timestamp are columns so retries with a newer revision can be
// compared without making timestamps part of semantic equality.
func (s TaskStatusSummary) SemanticJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(semanticPayload{
		PrimarySession:      s.PrimarySession,
		ForegroundActivity:  s.ForegroundActivity,
		ActiveSubagentCount: s.ActiveSubagentCount,
		PendingAction:       s.PendingAction,
		ActiveError:         s.ActiveError,
		Git:                 s.Git,
		PullRequest:         s.PullRequest,
	})
}

type semanticPayload struct {
	PrimarySession      *PrimarySessionSummary `json:"primary_session,omitempty"`
	ForegroundActivity  string                 `json:"foreground_activity,omitempty"`
	ActiveSubagentCount int                    `json:"active_subagent_count,omitempty"`
	PendingAction       string                 `json:"pending_action,omitempty"`
	ActiveError         *ActiveErrorSummary    `json:"active_error,omitempty"`
	Git                 *GitSummary            `json:"git,omitempty"`
	PullRequest         *PullRequestSummary    `json:"pull_request,omitempty"`
}
