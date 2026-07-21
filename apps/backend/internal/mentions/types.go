// Package mentions provides provider-neutral, workspace-scoped work-item search.
package mentions

import (
	"context"
	"errors"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	DefaultLimit = 5
	MaxLimit     = 10

	mentionProviderKandev    = "kandev"
	mentionKindTask          = "task"
	mentionKindIssue         = "issue"
	mentionLabelTask         = "Task"
	mentionLabelIssue        = "Issue"
	mentionLabelWorkItem     = "Work item"
	mentionIssuesPathSegment = "issues"
)

var (
	ErrInvalidDescriptor            = errors.New("invalid mention provider descriptor")
	ErrDuplicateSource              = errors.New("mention provider source already registered")
	ErrDuplicateProvider            = errors.New("mention provider and kind already registered")
	ErrMissingAuthorizer            = errors.New("mention provider reference authorizer is required")
	ErrInvalidRequest               = errors.New("invalid mention search request")
	ErrWorkspaceNotFound            = errors.New("mention search workspace not found")
	ErrReferenceProviderUnavailable = errors.New("reference provider unavailable")
	ErrReferenceUnauthorized        = errors.New("reference is not authorized")
)

type Status = apiv1.MentionStatus

const (
	StatusOK               = apiv1.MentionStatusOK
	StatusNotConfigured    = apiv1.MentionStatusNotConfigured
	StatusUnauthorized     = apiv1.MentionStatusUnauthorized
	StatusRateLimited      = apiv1.MentionStatusRateLimited
	StatusTimeout          = apiv1.MentionStatusTimeout
	StatusUpstreamError    = apiv1.MentionStatusUpstreamError
	StatusUnsupportedScope = apiv1.MentionStatusUnsupportedScope
)

// ProviderError classifies a provider failure without exposing its cause to clients.
type ProviderError struct {
	Status Status
	cause  error
}

func (e *ProviderError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return string(e.Status)
}

func (e *ProviderError) Unwrap() error {
	return e.cause
}

// NewProviderError wraps an optional diagnostic cause with a safe response status.
func NewProviderError(status Status, causes ...error) error {
	var cause error
	if len(causes) > 0 {
		cause = causes[0]
	}
	return &ProviderError{Status: status, cause: cause}
}

// ProviderDescriptor is immutable registry-owned identity and presentation metadata.
type ProviderDescriptor struct {
	Source      string
	Provider    string
	Kind        string
	DisplayName string
	KindLabel   string
	Order       int
}

// SearchRequest is the plain-text boundary shared by every provider.
type SearchRequest struct {
	WorkspaceID   string
	Query         string
	Limit         int
	ExcludeTaskID string
}

// Candidate is untrusted provider-local data. Registry identity is intentionally absent.
type Candidate struct {
	ID    string
	Key   string
	Title string
	URL   string
	Scope string
}

// ReferencePurpose lets one provider apply cheaper search checks and stronger submission checks.
type ReferencePurpose string

const (
	ReferencePurposeSearch     ReferencePurpose = "search"
	ReferencePurposeSubmission ReferencePurpose = "submission"
)

// ReferenceAuthorizationRequest contains trusted conversation scope and one normalized reference.
type ReferenceAuthorizationRequest struct {
	WorkspaceID string
	Purpose     ReferencePurpose
	Reference   apiv1.EntityReference
}

// ReferenceAuthorizer verifies provider-owned scope and destination rules.
type ReferenceAuthorizer interface {
	AuthorizeReference(context.Context, ReferenceAuthorizationRequest) error
}

// MentionProvider searches one provider source and kind.
type MentionProvider interface {
	Descriptor() ProviderDescriptor
	Search(context.Context, SearchRequest) ([]Candidate, error)
}
