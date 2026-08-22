package store

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

var (
	// ErrProfileChanged is returned by DuplicateAgentProfile when a source
	// row changed between the caller's read and the transactional insert, so
	// the copy would not reflect a consistent snapshot. Callers retry on a
	// fresh read.
	ErrProfileChanged = errors.New("source profile changed during duplicate")
	// ErrSourceProfileNotFound is returned by DuplicateAgentProfile when the
	// source profile row no longer exists (deleted mid-flight).
	ErrSourceProfileNotFound = errors.New("source profile not found")
)

// DuplicateAgentProfileInput carries everything the atomic duplicate needs:
// the copy rows the caller built (Profile, McpConfig) plus the source rows
// they were built from (Source, SourceMcp) so the transaction can verify the
// copy reflects one consistent snapshot.
type DuplicateAgentProfileInput struct {
	Source    *models.AgentProfile
	SourceMcp *models.AgentProfileMcpConfig // nil when the source has no MCP row
	Profile   *models.AgentProfile
	McpConfig *models.AgentProfileMcpConfig // nil when SourceMcp is nil
}

type Repository interface {
	CreateAgent(ctx context.Context, agent *models.Agent) error
	GetAgent(ctx context.Context, id string) (*models.Agent, error)
	GetAgentByName(ctx context.Context, name string) (*models.Agent, error)
	UpdateAgent(ctx context.Context, agent *models.Agent) error
	DeleteAgent(ctx context.Context, id string) error
	ListAgents(ctx context.Context) ([]*models.Agent, error)
	ListTUIAgents(ctx context.Context) ([]*models.Agent, error)

	GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*models.AgentProfileMcpConfig, error)
	UpsertAgentProfileMcpConfig(ctx context.Context, config *models.AgentProfileMcpConfig) error

	CreateAgentProfile(ctx context.Context, profile *models.AgentProfile) error
	// DuplicateAgentProfile creates an independent copy of a profile in one
	// transaction: the row is inserted with the caller-provided Enabled state
	// and the MCP config row is upserted when non-nil. A failure rolls back
	// and leaves no partial copy. The source rows are re-read inside the
	// transaction and must still match the revisions the copy was built from;
	// otherwise ErrProfileChanged is returned and no row is created.
	DuplicateAgentProfile(ctx context.Context, input DuplicateAgentProfileInput) error
	UpdateAgentProfile(ctx context.Context, profile *models.AgentProfile) error
	UpdateAgentProfileEnabled(ctx context.Context, id string, enabled bool) (time.Time, error)
	DeleteAgentProfile(ctx context.Context, id string) error
	GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error)
	// GetAgentProfileIncludingDeleted returns the row even when soft-deleted.
	// Check profile.DeletedAt != nil to detect orphaned references (watchers,
	// automations) pointing at removed profiles. ErrAgentProfileDeleted is
	// only used by callers of ProfileResolver, which wraps this method.
	GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error)
	// GetAgentProfileTx is GetAgentProfile's transaction-accepting counterpart:
	// the automations YAML export (AC-29) opens one read transaction spanning
	// several stores and passes it through here rather than letting this
	// method open its own, so the profile read observes the same snapshot as
	// every other read in the export. A missing row is reported as
	// found=false rather than an error - AC-19's partial-resolution rule
	// decides what that means, not this method.
	GetAgentProfileTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.AgentProfile, bool, error)
	ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error)
	// HasDeletedAgentProfiles reports whether the agent has any soft-deleted
	// profile rows. Seeding paths use this to distinguish a fresh agent that
	// has never been provisioned (no rows at all -> seed a default) from one
	// whose profiles the user deliberately deleted (deleted rows present ->
	// do not resurrect them on the next boot).
	HasDeletedAgentProfiles(ctx context.Context, agentID string) (bool, error)

	Close() error
}

// DynamicProfileRepository is the optional extension implemented by settings
// stores that persist the dynamic profile document. Keeping it separate from
// Repository lets small controller fakes and plugin adapters retain the
// existing profile contract while dynamic routing is rolled out.
type DynamicProfileRepository interface {
	CreateDynamicAgentProfile(ctx context.Context, profile *models.DynamicAgentProfile, routes []models.DynamicAgentRoute) error
	GetDynamicAgentProfile(ctx context.Context, profileID string) (*models.DynamicAgentProfile, []models.DynamicAgentRoute, error)
	UpdateDynamicAgentProfile(ctx context.Context, profile *models.DynamicAgentProfile, expectedVersion int64, routes []models.DynamicAgentRoute) error
	ListDynamicProfileReferencesByExecutionProfile(ctx context.Context, profileID string) ([]models.DynamicProfileReference, error)
}

// AtomicDynamicProfileRepository updates the base profile row and its
// optimistic-routing document in one transaction. Controllers use this
// optional extension when a request changes both surfaces so a stale route
// version cannot leave the base profile partially updated.
type AtomicDynamicProfileRepository interface {
	UpdateAgentProfileWithDynamic(
		ctx context.Context,
		profile *models.AgentProfile,
		dynamic *models.DynamicAgentProfile,
		expectedVersion int64,
		routes []models.DynamicAgentRoute,
	) error
}
