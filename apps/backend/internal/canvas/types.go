// Package canvas owns the lifecycle metadata for agent-authored web-app
// canvases. Plugin instance scope, status, releases, and cleanup inventory
// remain authoritative in internal/plugins/instances.
package canvas

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
)

const (
	// CanvasPluginID identifies the synthetic plugin binding used by local
	// canvas instances. The package identity remains in the release metadata.
	CanvasPluginID = "kandev.canvas"
	MaxTitleLength = 200

	MaxTaskCanvases      = plugininstances.MaxTaskInstances
	MaxWorkspaceCanvases = plugininstances.MaxWorkspaceInstances
)

const (
	ScopeTask      = plugininstances.ScopeTask
	ScopeWorkspace = plugininstances.ScopeWorkspace

	StatusPending  = plugininstances.StatusPending
	StatusActive   = plugininstances.StatusActive
	StatusDisabled = plugininstances.StatusDisabled
	StatusArchived = plugininstances.StatusArchived
	StatusError    = plugininstances.StatusError
	StatusRemoved  = plugininstances.StatusRemoved

	ValidationValid             = plugininstances.ValidationValid
	ValidationPendingPermission = plugininstances.ValidationPendingPermission
	ValidationInvalid           = plugininstances.ValidationInvalid
	ValidationUnavailable       = plugininstances.ValidationUnavailable
)

const (
	permissionKindAPIRead  = "api_read"
	permissionKindAPIWrite = "api_write"
	permissionKindEvents   = "events"
	permissionKindNetwork  = "network"
	permissionKindState    = "state"
)

var (
	ErrCanvasNotFound        = errors.New("canvas not found")
	ErrInvalidCanvas         = errors.New("invalid canvas")
	ErrInvalidCanvasState    = errors.New("invalid canvas state")
	ErrCanvasMetadataBroken  = errors.New("canvas metadata is inconsistent with its plugin instance")
	ErrCanvasNotConfigured   = errors.New("canvas lifecycle is not configured")
	ErrStalePromotionReview  = plugininstances.ErrStalePromotionReview
	ErrStaleCanvasEdit       = plugininstances.ErrStaleCanvasEdit
	ErrStaleCanvasPublish    = plugininstances.ErrStaleCanvasPublish
	ErrInvalidLifecycleState = plugininstances.ErrInvalidLifecycleState

	// These aliases preserve the stable plugin admission errors while keeping
	// the canvas package convenient for service callers and API adapters.
	ErrTaskCanvasLimit      = plugininstances.ErrTaskCanvasLimit
	ErrWorkspaceCanvasLimit = plugininstances.ErrWorkspaceCanvasLimit
)

// CreateCanvasRequest contains trusted scope metadata supplied by the caller.
// Agent-facing entry points must populate it from their execution context;
// the canvas service does not infer or authorize arbitrary task IDs.
type CreateCanvasRequest struct {
	WorkspaceID        string `json:"workspace_id"`
	TaskID             string `json:"task_id,omitempty"`
	OriginTaskID       string `json:"origin_task_id,omitempty"`
	Title              string `json:"title"`
	CreatedBySessionID string `json:"created_by_session_id,omitempty"`
	PluginID           string `json:"plugin_id,omitempty"`
}

// Canvas is the lifecycle projection consumed by host, API, and event layers.
// Scope, status, and active release fields come from the plugin instance; the
// remaining fields are canvas-owned metadata.
type Canvas struct {
	ID                  string            `json:"id"`
	PluginInstanceID    string            `json:"plugin_instance_id"`
	PluginID            string            `json:"plugin_id"`
	WorkspaceID         string            `json:"workspace_id"`
	TaskID              string            `json:"task_id,omitempty"`
	OriginTaskID        string            `json:"origin_task_id,omitempty"`
	ScopeKind           string            `json:"scope_kind"`
	Title               string            `json:"title"`
	CreatedBySessionID  string            `json:"created_by_session_id,omitempty"`
	PromotedByUserID    string            `json:"promoted_by_user_id,omitempty"`
	PromotedAt          *time.Time        `json:"promoted_at,omitempty"`
	Status              string            `json:"status"`
	ActiveReleaseID     string            `json:"active_release_id,omitempty"`
	ActiveReleaseStatus string            `json:"active_release_status,omitempty"`
	ActiveReleaseError  string            `json:"active_release_error,omitempty"`
	GrantGeneration     int64             `json:"grant_generation,omitempty"`
	EffectiveGrants     []GrantProjection `json:"effective_grants,omitempty"`
	ActiveRelease       *ReleaseMetadata  `json:"active_release,omitempty"`
	PendingRelease      *ReleaseMetadata  `json:"pending_release,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// ReleaseMetadata deliberately omits manifest and artifact contents. The host
// receives only the review-safe declaration, provenance, and current grant
// diff needed to choose recovery actions.
type ReleaseMetadata struct {
	ID                 string             `json:"id"`
	PackageDigest      string             `json:"package_digest"`
	ValidationStatus   string             `json:"validation_status"`
	ValidationError    string             `json:"validation_error,omitempty"`
	Permissions        *PermissionSummary `json:"permissions,omitempty"`
	MissingPermissions []string           `json:"missing_permissions,omitempty"`
	PermissionDigest   string             `json:"permission_digest,omitempty"`
	SourceActorKind    string             `json:"source_actor_kind,omitempty"`
	SourceUserID       string             `json:"source_user_id,omitempty"`
	SourceTaskID       string             `json:"source_task_id,omitempty"`
	SourceSessionID    string             `json:"source_session_id,omitempty"`
	ProtocolVersion    int                `json:"protocol_version,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
}

// GrantProjection is the safe, non-credential view of one effective grant.
// It intentionally excludes approver identity and timestamps.
type GrantProjection struct {
	PermissionKind string `json:"permission_kind"`
	Resource       string `json:"resource,omitempty"`
	NetworkOrigin  string `json:"network_origin,omitempty"`
	ScopeCeiling   string `json:"scope_ceiling"`
}

// CanvasMetadata is the canvas-owned half of a lifecycle record. The plugin
// instance table is intentionally not duplicated here.
type CanvasMetadata struct {
	ID                 string     `json:"id"`
	PluginInstanceID   string     `json:"plugin_instance_id"`
	WorkspaceID        string     `json:"workspace_id"`
	TaskID             string     `json:"task_id,omitempty"`
	OriginTaskID       string     `json:"origin_task_id,omitempty"`
	Title              string     `json:"title"`
	CreatedBySessionID string     `json:"created_by_session_id,omitempty"`
	PromotedByUserID   string     `json:"promoted_by_user_id,omitempty"`
	PromotedAt         *time.Time `json:"promoted_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// PluginInstanceStore is the small adapter surface used by the lifecycle
// service. *instances.Store satisfies it, while a narrow fake can be used by
// service tests or a later transactional integration.
type PluginInstanceStore interface {
	Create(context.Context, plugininstances.Instance) error
	Get(context.Context, string) (plugininstances.Instance, error)
	GetRelease(context.Context, string) (plugininstances.Release, error)
	Archive(context.Context, string) error
	Restore(context.Context, string) error
	RemoveInstance(context.Context, string) error
}

// transactionalPluginInstanceStore is the durable integration surface used
// when canvas metadata and plugin-instance authority must commit together.
// It is optional so small lifecycle fakes remain useful in unit tests.
type transactionalPluginInstanceStore interface {
	PluginInstanceStore
	WithTransaction(context.Context, func(*sqlx.Tx) error) error
	CreateTx(context.Context, *sqlx.Tx, plugininstances.Instance) error
	PromoteScopeAndGrantsTx(context.Context, *sqlx.Tx, string, string, string, []plugininstances.Grant) error
	RemoveInstanceTx(context.Context, *sqlx.Tx, string) error
	ListBySource(context.Context, string, bool) ([]plugininstances.Instance, error)
}

// LifecycleEvent is a content-free notification for the WebSocket gateway.
// It never carries source files, application state, package bodies, or runtime
// capabilities.
type LifecycleEvent struct {
	Type                string `json:"type"`
	CanvasID            string `json:"canvas_id"`
	PluginInstanceID    string `json:"plugin_instance_id"`
	WorkspaceID         string `json:"workspace_id"`
	TaskID              string `json:"task_id,omitempty"`
	ScopeKind           string `json:"scope_kind"`
	Status              string `json:"status"`
	ActiveReleaseID     string `json:"active_release_id,omitempty"`
	ActiveReleaseStatus string `json:"active_release_status,omitempty"`
}

const (
	EventCreated                   = "canvas.created"
	EventReleaseActivated          = "canvas.release.activated"
	EventReleasePermissionRequired = "canvas.release.permission_required"
	EventPromoted                  = "canvas.promoted"
	EventArchived                  = "canvas.archived"
	EventRestored                  = "canvas.restored"
	EventRemoved                   = "canvas.removed"
)

// EventPublisher receives committed lifecycle changes. Publishing is
// best-effort and must be wired to an out-of-transaction event gateway.
type EventPublisher func(context.Context, LifecycleEvent)
