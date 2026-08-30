package github

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	rateLimitBlockSecondary          = "observed_secondary_rate_limit"
	rateLimitBlockPrimary            = "primary_rate_limit"
	rateLimitBlockPrimaryReserve     = "primary_reserve"
	rateLimitBlockBackgroundBusy     = "background_in_flight"
	rateLimitBlockInteractiveWaiting = "interactive_priority"
	rateLimitBlockBackgroundPacing   = "background_pacing"
)

// RateLimitPrincipalSnapshot identifies the non-secret upstream quota owner
// whose observations and admission decisions are shared across workspaces.
type RateLimitPrincipalSnapshot struct {
	Kind              AuthPrincipalKind `json:"kind"`
	Source            ConnectionSource  `json:"source"`
	Host              string            `json:"host"`
	Login             string            `json:"login,omitempty"`
	AppRegistrationID string            `json:"app_registration_id,omitempty"`
	InstallationID    int64             `json:"installation_id,omitempty"`
}

// RateLimitBucketSnapshot is Kandev's last local observation of one primary
// GitHub rate bucket. Fresh means the observation belongs to a rate window
// whose reset time has not passed; it does not imply a provider-side refresh.
type RateLimitBucketSnapshot struct {
	Resource   Resource   `json:"resource"`
	Known      bool       `json:"known"`
	Fresh      bool       `json:"fresh"`
	Limit      int        `json:"limit,omitempty"`
	Remaining  int        `json:"remaining,omitempty"`
	ResetAt    *time.Time `json:"reset_at,omitempty"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

// ObservedSecondaryRateLimitSnapshot is explicitly Kandev-observed state.
// RetryAt is the local enforcement boundary, never an authoritative GitHub
// secondary-limit clear time.
type ObservedSecondaryRateLimitSnapshot struct {
	Active      bool        `json:"active"`
	Resource    Resource    `json:"resource,omitempty"`
	RetryAt     *time.Time  `json:"retry_at,omitempty"`
	ObservedAt  *time.Time  `json:"observed_at,omitempty"`
	RetrySource RetrySource `json:"retry_source,omitempty"`
	Reason      string      `json:"reason,omitempty"`
}

// WorkspaceRateLimitSnapshot is a provider-free view of local observations
// and the admission decisions currently enforced for the workspace principal.
type WorkspaceRateLimitSnapshot struct {
	WorkspaceID        string                             `json:"workspace_id"`
	Principal          RateLimitPrincipalSnapshot         `json:"principal"`
	Core               RateLimitBucketSnapshot            `json:"core"`
	GraphQL            RateLimitBucketSnapshot            `json:"graphql"`
	Search             RateLimitBucketSnapshot            `json:"search"`
	Secondary          ObservedSecondaryRateLimitSnapshot `json:"observed_secondary"`
	InteractiveAllowed bool                               `json:"interactive_allowed"`
	BackgroundAllowed  bool                               `json:"background_allowed"`
	BlockingReason     string                             `json:"blocking_reason,omitempty"`
}

type rateAdmissionDecision struct {
	interactiveAllowed bool
	backgroundAllowed  bool
	interactiveReason  string
	backgroundReason   string
}

// GetWorkspaceRateLimitSnapshot reads only persisted connection metadata and
// in-memory coordinator state. It deliberately never resolves credentials,
// calls /rate_limit, or sends any other provider request.
func (s *Service) GetWorkspaceRateLimitSnapshot(
	ctx context.Context,
	workspaceID string,
) (WorkspaceRateLimitSnapshot, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return WorkspaceRateLimitSnapshot{}, err
	}
	if s == nil || s.store == nil {
		return WorkspaceRateLimitSnapshot{}, ErrGitHubNotConfigured
	}
	connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return WorkspaceRateLimitSnapshot{}, fmt.Errorf("get GitHub workspace connection: %w", err)
	}
	if connection == nil {
		return WorkspaceRateLimitSnapshot{}, ErrGitHubNotConfigured
	}

	host, principal := rateLimitPrincipalFromConnection(connection)
	tracker, admission := s.rateCoordinator.coordinate(host, principal, nil)
	now := time.Now().UTC()
	coreDecision := admission.snapshot(ResourceCore, now)
	graphqlDecision := admission.snapshot(ResourceGraphQL, now)
	searchDecision := admission.snapshot(ResourceSearch, now)
	interactiveAllowed, interactiveReason := combineRateAdmission(
		coreDecision.interactiveAllowed, coreDecision.interactiveReason,
		graphqlDecision.interactiveAllowed, graphqlDecision.interactiveReason,
		searchDecision.interactiveAllowed, searchDecision.interactiveReason,
	)
	backgroundAllowed, backgroundReason := combineRateAdmission(
		coreDecision.backgroundAllowed, coreDecision.backgroundReason,
		graphqlDecision.backgroundAllowed, graphqlDecision.backgroundReason,
		searchDecision.backgroundAllowed, searchDecision.backgroundReason,
	)
	blockingReason := interactiveReason
	if blockingReason == "" {
		blockingReason = backgroundReason
	}

	return WorkspaceRateLimitSnapshot{
		WorkspaceID: workspaceID,
		Principal: RateLimitPrincipalSnapshot{
			Kind: principal.Kind, Source: principal.Source, Host: host,
			Login: principal.Login, AppRegistrationID: principal.AppRegistrationID,
			InstallationID: principal.InstallationID,
		},
		Core:               rateLimitBucketSnapshot(tracker, ResourceCore, now),
		GraphQL:            rateLimitBucketSnapshot(tracker, ResourceGraphQL, now),
		Search:             rateLimitBucketSnapshot(tracker, ResourceSearch, now),
		Secondary:          observedSecondarySnapshot(tracker, now),
		InteractiveAllowed: interactiveAllowed,
		BackgroundAllowed:  backgroundAllowed,
		BlockingReason:     blockingReason,
	}, nil
}

func rateLimitPrincipalFromConnection(connection *WorkspaceConnection) (string, AuthPrincipal) {
	host := strings.ToLower(strings.TrimSpace(connection.GitHubHost))
	if host == "" {
		host = defaultGitHubHost
	}
	principal := AuthPrincipal{
		Kind: AuthPrincipalHuman, Source: connection.Source,
		Login: strings.TrimSpace(connection.Login), WorkspaceID: connection.WorkspaceID,
	}
	if connection.Source == ConnectionSourceGitHubAppInstallation && connection.InstallationID != nil {
		principal.Kind = AuthPrincipalApp
		principal.AppRegistrationID = connection.AppRegistrationID
		principal.InstallationID = *connection.InstallationID
	}
	if connection.Source == ConnectionSourceLegacyShared && principal.Login == "" {
		principal.WorkspaceID = "legacy"
	}
	return host, principal
}

func rateLimitBucketSnapshot(tracker *RateTracker, resource Resource, now time.Time) RateLimitBucketSnapshot {
	result := RateLimitBucketSnapshot{Resource: resource}
	snapshot, known := tracker.Snapshot(resource)
	if !known {
		return result
	}
	resetAt := snapshot.ResetAt
	observedAt := snapshot.UpdatedAt
	result.Known = true
	result.Fresh = !observedAt.IsZero() && resetAt.After(now)
	result.Limit = snapshot.Limit
	result.Remaining = snapshot.Remaining
	result.ResetAt = &resetAt
	result.ObservedAt = &observedAt
	return result
}

func observedSecondarySnapshot(tracker *RateTracker, now time.Time) ObservedSecondaryRateLimitSnapshot {
	states := []SecondaryRateLimitState{
		tracker.Secondary(ResourceCore),
		tracker.Secondary(ResourceGraphQL),
		tracker.Secondary(ResourceSearch),
	}
	var selected SecondaryRateLimitState
	for _, state := range states {
		if state.ObservedAt.IsZero() {
			continue
		}
		state.Active = state.RetryAt.After(now)
		if selected.ObservedAt.IsZero() ||
			(state.Active && !selected.Active) ||
			(state.Active && selected.Active && state.RetryAt.After(selected.RetryAt)) ||
			(!state.Active && !selected.Active && state.ObservedAt.After(selected.ObservedAt)) {
			selected = state
		}
	}
	if selected.ObservedAt.IsZero() {
		return ObservedSecondaryRateLimitSnapshot{}
	}
	retryAt := selected.RetryAt
	observedAt := selected.ObservedAt
	return ObservedSecondaryRateLimitSnapshot{
		Active: selected.Active, Resource: selected.Resource,
		RetryAt: &retryAt, ObservedAt: &observedAt,
		RetrySource: selected.RetrySource, Reason: rateLimitBlockSecondary,
	}
}

func combineRateAdmission(
	firstAllowed bool,
	firstReason string,
	secondAllowed bool,
	secondReason string,
	thirdAllowed bool,
	thirdReason string,
) (bool, string) {
	if !firstAllowed {
		return false, firstReason
	}
	if !secondAllowed {
		return false, secondReason
	}
	if !thirdAllowed {
		return false, thirdReason
	}
	return true, ""
}
