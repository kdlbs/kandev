package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

// DeletedProfileError carries the soft-deleted profile's ID and name so the
// caller (watcher self-heal) can surface a human-readable cause. Wraps
// store.ErrAgentProfileDeleted — use errors.Is / errors.As.
type DeletedProfileError struct {
	ProfileID   string
	ProfileName string
}

func (e *DeletedProfileError) Error() string {
	if e.ProfileName != "" {
		return fmt.Sprintf("agent profile %q (%s) was removed", e.ProfileName, e.ProfileID)
	}
	return fmt.Sprintf("agent profile %s was removed", e.ProfileID)
}

func (e *DeletedProfileError) Unwrap() error { return store.ErrAgentProfileDeleted }

// StoreProfileResolver implements ProfileResolver using the agent settings store
type StoreProfileResolver struct {
	store    store.Repository
	registry *registry.Registry
}

// NewStoreProfileResolver creates a new profile resolver using the given store and registry.
// The registry is used to look up the agent's default model when the profile doesn't specify one.
func NewStoreProfileResolver(store store.Repository, reg *registry.Registry) *StoreProfileResolver {
	return &StoreProfileResolver{store: store, registry: reg}
}

// ResolveProfile looks up an agent profile by ID and returns the profile info.
//
// When the row is missing, ResolveProfile retries via
// GetAgentProfileIncludingDeleted to disambiguate "never existed" (returns
// the original "profile not found" error) from "soft-deleted" (returns a
// *DeletedProfileError wrapping store.ErrAgentProfileDeleted so callers can
// trigger watcher self-heal).
func (r *StoreProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	profile, err := r.store.GetAgentProfile(ctx, profileID)
	if err != nil {
		if deletedErr := r.checkSoftDeleted(ctx, profileID, err); deletedErr != nil {
			return nil, deletedErr
		}
		return nil, fmt.Errorf("profile not found: %w", err)
	}

	// Get the parent agent to get the agent name
	agent, err := r.store.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found for profile: %w", err)
	}

	// Resolve agent capabilities from the registry.
	model, nativeSessionResume := r.resolveAgentCapabilities(agent.Name, profile.Model)

	return &AgentProfileInfo{
		ProfileID:                  profile.ID,
		ProfileName:                profile.Name,
		AgentID:                    agent.ID,
		AgentName:                  agent.Name,
		Model:                      model,
		Mode:                       profile.Mode,
		AutoApprove:                profile.AutoApprove,
		DangerouslySkipPermissions: profile.DangerouslySkipPermissions,
		AllowIndexing:              profile.AllowIndexing,
		CLIFlags:                   profile.CLIFlags,
		EnvVars:                    profile.EnvVars,
		CLIPassthrough:             profile.CLIPassthrough,
		NativeSessionResume:        nativeSessionResume,
		SupportsMCP:                agent.SupportsMCP,
	}, nil
}

// checkSoftDeleted returns a *DeletedProfileError when the missing-row error
// from GetAgentProfile resolves to an existing-but-soft-deleted row. Returns
// nil when the row really doesn't exist or the secondary lookup fails — the
// caller falls back to the original "profile not found" error.
func (r *StoreProfileResolver) checkSoftDeleted(ctx context.Context, profileID string, primaryErr error) error {
	// Only retry on the well-known "missing row" signal. The store layer
	// returns sql.ErrNoRows from QueryRow.Scan when the filtered SELECT
	// matches nothing — that includes both "no row at all" and
	// "row exists but deleted_at IS NOT NULL". Other errors (driver,
	// permissions) are returned as-is by the caller.
	if !errors.Is(primaryErr, sql.ErrNoRows) {
		return nil
	}
	profile, err := r.store.GetAgentProfileIncludingDeleted(ctx, profileID)
	if err != nil || profile == nil || profile.DeletedAt == nil {
		return nil
	}
	return &DeletedProfileError{ProfileID: profile.ID, ProfileName: profile.Name}
}

// resolveAgentCapabilities looks up the agent in the registry and returns the
// effective model and whether the agent supports native session resume.
// The model comes straight from the profile; static per-agent defaults have
// been removed. Empty model means "agent picks its own default".
func (r *StoreProfileResolver) resolveAgentCapabilities(agentName, profileModel string) (string, bool) {
	if r.registry == nil {
		return profileModel, false
	}
	ag, ok := r.registry.Get(agentName)
	if !ok {
		return profileModel, false
	}
	var nativeSessionResume bool
	if rt := ag.Runtime(); rt != nil {
		nativeSessionResume = rt.SessionConfig.NativeSessionResume
	}
	return profileModel, nativeSessionResume
}
