package canvas

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

// PermissionSummary is the review-safe projection of a canvas manifest. It
// contains permission names and exact origins, never package source or state.
type PermissionSummary struct {
	Reads           []string `json:"reads,omitempty"`
	Writes          []string `json:"writes,omitempty"`
	Events          []string `json:"events,omitempty"`
	SharedState     bool     `json:"shared_state"`
	ExternalOrigins []string `json:"external_origins,omitempty"`
}

// PublishRequest contains a package already read from the trusted execution
// source stream. The request has no user-controlled scope identifiers.
type PublishRequest struct {
	CanvasID string
	Package  *webapp.Package
	Artifact webapp.Artifact
	// ExpectedAuthority is captured by the trusted authoring boundary before
	// source transfer. Durable persistence must still match it atomically.
	ExpectedAuthority plugininstances.PublishAuthority
	// ExpectedBaseReleaseID is set for a trusted edit session. The release
	// publish transaction rejects the package if the active release changed
	// after the editor materialized its source.
	ExpectedBaseReleaseID string
	SourceActorKind       string
	SourceUserID          string
	SourceTaskID          string
	SourceSessionID       string
}

// PublishResult describes the release outcome without exposing package files.
type PublishResult struct {
	Canvas             *Canvas
	Release            plugininstances.Release
	Activated          bool
	PermissionRequired bool
	// ReleasePersisted tells the source owner that the artifact is now owned
	// by durable release metadata, even when a post-persistence projection or
	// retention step returned an error.
	ReleasePersisted bool
}

// PromotionPreview is shown to a human before a task canvas becomes a
// workspace canvas.
type PromotionPreview struct {
	Canvas           *Canvas
	SourceActorKind  string            `json:"source_actor_kind"`
	SourceUserID     string            `json:"source_user_id,omitempty"`
	SourceTaskID     string            `json:"source_task_id,omitempty"`
	SourceSessionID  string            `json:"source_session_id,omitempty"`
	Permissions      PermissionSummary `json:"permissions"`
	ActiveReleaseID  string            `json:"active_release_id"`
	PermissionDigest string            `json:"permission_digest"`
	GrantGeneration  int64             `json:"grant_generation"`
	CurrentScope     string            `json:"current_scope"`
	TargetScope      string            `json:"target_scope"`
	Placement        string            `json:"placement"`
}

// authoringInstanceStore is the release/governance extension implemented by
// the durable plugin instance store. Keeping it optional preserves the small
// lifecycle fake used by the basic canvas service tests.
type authoringInstanceStore interface {
	PluginInstanceStore
	SetPluginID(context.Context, string, string) error
	CreateRelease(context.Context, plugininstances.Release) error
	ListReleases(context.Context, string) ([]plugininstances.Release, error)
	ActivateRelease(context.Context, string, string) error
	SetReleaseValidation(context.Context, string, string, string) error
	ListGrants(context.Context, string) ([]plugininstances.Grant, error)
	PromoteScopeAndGrants(context.Context, string, string, string, []plugininstances.Grant) error
	ApproveRelease(context.Context, string, string, string, []plugininstances.Grant) error
}

type transactionalAuthoringStore interface {
	authoringInstanceStore
	WithTransaction(context.Context, func(*sqlx.Tx) error) error
	CreateReleaseTx(context.Context, *sqlx.Tx, plugininstances.Release) error
	SetPluginIDTx(context.Context, *sqlx.Tx, string, string) error
	ActivateReleaseTx(context.Context, *sqlx.Tx, string, string) error
}

type reviewedPromotionStore interface {
	PromoteScopeAndGrantsReviewedTx(context.Context, *sqlx.Tx, string, string, string, []plugininstances.Grant, string, string, int64) error
}

type conditionalReleaseStore interface {
	CreateReleaseIfAuthorityTx(context.Context, *sqlx.Tx, string, plugininstances.PublishAuthority, plugininstances.Release) error
}

type legacyConditionalReleaseStore interface {
	CreateReleaseIfActiveReleaseTx(context.Context, *sqlx.Tx, string, string, plugininstances.Release) error
}

// releaseRetentionStore is optional so the authoring service remains
// compatible with the narrow fakes used by lifecycle tests. The durable
// plugin instance store implements it to remove superseded release ownership
// after a publish.
type releaseRetentionStore interface {
	PruneReleases(context.Context, string) error
}

type releaseRejectionStore interface {
	RejectReleaseAndPrune(context.Context, string, string) error
}

// PublishPackage stores one validated package as an immutable release. A
// failed validation, quota check, or activation leaves the current active
// pointer untouched.
func (s *Service) PublishPackage(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	instance, release, activated, err := s.preparePublishedRelease(ctx, store, request)
	if err != nil {
		return nil, err
	}
	persisted, err := persistPublishedRelease(ctx, store, instance.ID, release, activated, request.ExpectedAuthority, request.ExpectedBaseReleaseID)
	if err != nil {
		if persisted {
			return &PublishResult{Release: release, Activated: activated, PermissionRequired: !activated, ReleasePersisted: true}, err
		}
		return nil, err
	}
	result := &PublishResult{Release: release, Activated: activated, PermissionRequired: !activated, ReleasePersisted: true}
	if err := prunePublishedReleases(ctx, store, instance.ID); err != nil {
		return result, err
	}
	resultCanvas, err := s.Get(ctx, request.CanvasID)
	if err != nil {
		return result, err
	}
	result.Canvas = resultCanvas
	eventType := EventReleasePermissionRequired
	if activated {
		eventType = EventReleaseActivated
	}
	s.mu.Lock()
	publisher := s.publisher
	s.mu.Unlock()
	publishEvent(ctx, publisher, lifecycleEvent(eventType, *resultCanvas))
	return result, nil
}

func (s *Service) preparePublishedRelease(ctx context.Context, store authoringInstanceStore, request PublishRequest) (plugininstances.Instance, plugininstances.Release, bool, error) {
	if request.Package == nil || request.Package.Manifest == nil || request.Artifact.Digest == "" {
		return plugininstances.Instance{}, plugininstances.Release{}, false, fmt.Errorf("%w: package and artifact are required", ErrInvalidCanvas)
	}
	_, instance, err := s.load(ctx, request.CanvasID)
	if err != nil {
		return plugininstances.Instance{}, plugininstances.Release{}, false, err
	}
	if instance.Status == StatusRemoved || instance.Status == StatusArchived {
		return plugininstances.Instance{}, plugininstances.Release{}, false, fmt.Errorf("%w: canvas is %s", ErrInvalidCanvasState, instance.Status)
	}
	if !request.ExpectedAuthority.IsZero() && request.ExpectedAuthority != instance.PublishAuthority() {
		return plugininstances.Instance{}, plugininstances.Release{}, false, ErrStaleCanvasPublish
	}
	if request.Artifact.Digest != request.Package.Digest {
		return plugininstances.Instance{}, plugininstances.Release{}, false, fmt.Errorf("%w: artifact digest does not match package", ErrInvalidCanvas)
	}
	if err := request.Package.Manifest.Validate(); err != nil {
		return plugininstances.Instance{}, plugininstances.Release{}, false, fmt.Errorf("%w: invalid manifest: %v", ErrInvalidCanvas, err)
	}
	permissions := ManifestPermissions(request.Package.Manifest)
	grants, err := store.ListGrants(ctx, instance.ID)
	if err != nil {
		return plugininstances.Instance{}, plugininstances.Release{}, false, err
	}
	activated := permissionsFit(permissions, instance.ScopeKind, grants)
	validationStatus, validationError := plugininstances.ValidationPendingPermission, "permission_review_required"
	if activated {
		validationStatus, validationError = plugininstances.ValidationValid, ""
	}
	manifestJSON, err := json.Marshal(request.Package.Manifest)
	if err != nil {
		return plugininstances.Instance{}, plugininstances.Release{}, false, fmt.Errorf("marshal canvas manifest: %w", err)
	}
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return plugininstances.Instance{}, plugininstances.Release{}, false, fmt.Errorf("marshal canvas permissions: %w", err)
	}
	release := plugininstances.Release{
		ID: uuid.NewString(), PluginID: request.Package.Manifest.ID, InstanceID: instance.ID,
		PackageDigest: request.Package.Digest, SourceKind: plugininstances.SourceLocalCanvas,
		SourceActorKind: strings.TrimSpace(request.SourceActorKind), SourceUserID: strings.TrimSpace(request.SourceUserID),
		SourceTaskID: strings.TrimSpace(request.SourceTaskID), SourceSessionID: strings.TrimSpace(request.SourceSessionID),
		ManifestJSON: manifestJSON, DeclaredPermissionsJSON: permissionsJSON,
		ArtifactPath: request.Artifact.RelativePath, ArtifactBytes: request.Artifact.Bytes,
		ProtocolVersion: 1, ValidationStatus: validationStatus, ValidationError: validationError,
		CreatedAt: s.nowUTC(),
	}
	return instance, release, activated, nil
}

func persistPublishedRelease(ctx context.Context, store authoringInstanceStore, instanceID string, release plugininstances.Release, activated bool, expectedAuthority plugininstances.PublishAuthority, expectedBaseReleaseID string) (bool, error) {
	if !expectedAuthority.IsZero() {
		if expectedBaseReleaseID != "" && expectedBaseReleaseID != expectedAuthority.ActiveReleaseID {
			return false, ErrStaleCanvasEdit
		}
		transactional, ok := store.(transactionalAuthoringStore)
		conditional, supportsConditional := store.(conditionalReleaseStore)
		if !ok || !supportsConditional {
			return false, ErrStaleCanvasPublish
		}
		err := persistAuthorityRelease(ctx, transactional, conditional, instanceID, release, activated, expectedAuthority)
		return err == nil, err
	}
	if expectedBaseReleaseID != "" {
		transactional, ok := store.(transactionalAuthoringStore)
		conditional, supportsConditional := store.(legacyConditionalReleaseStore)
		if !ok || !supportsConditional {
			return false, ErrStaleCanvasEdit
		}
		err := persistConditionalRelease(ctx, transactional, conditional, instanceID, release, activated, expectedBaseReleaseID)
		return err == nil, err
	}
	if !activated {
		if err := store.CreateRelease(ctx, release); err != nil {
			return false, err
		}
		return true, nil
	}
	transactional, ok := store.(transactionalAuthoringStore)
	if ok {
		err := persistActivatedRelease(ctx, transactional, instanceID, release)
		return err == nil, err
	}
	return persistActivatedReleaseFallback(ctx, store, instanceID, release)
}

func persistAuthorityRelease(ctx context.Context, transactional transactionalAuthoringStore, conditional conditionalReleaseStore, instanceID string, release plugininstances.Release, activated bool, expectedAuthority plugininstances.PublishAuthority) error {
	return transactional.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		if err := conditional.CreateReleaseIfAuthorityTx(ctx, tx, instanceID, expectedAuthority, release); err != nil {
			return err
		}
		if !activated {
			return nil
		}
		if err := transactional.SetPluginIDTx(ctx, tx, instanceID, release.PluginID); err != nil {
			return err
		}
		return transactional.ActivateReleaseTx(ctx, tx, instanceID, release.ID)
	})
}

func persistConditionalRelease(ctx context.Context, transactional transactionalAuthoringStore, conditional legacyConditionalReleaseStore, instanceID string, release plugininstances.Release, activated bool, expectedBaseReleaseID string) error {
	return transactional.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		if err := conditional.CreateReleaseIfActiveReleaseTx(ctx, tx, instanceID, expectedBaseReleaseID, release); err != nil {
			return err
		}
		if !activated {
			return nil
		}
		if err := transactional.SetPluginIDTx(ctx, tx, instanceID, release.PluginID); err != nil {
			return err
		}
		return transactional.ActivateReleaseTx(ctx, tx, instanceID, release.ID)
	})
}

func persistActivatedRelease(ctx context.Context, store transactionalAuthoringStore, instanceID string, release plugininstances.Release) error {
	return store.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		if err := store.CreateReleaseTx(ctx, tx, release); err != nil {
			return err
		}
		if err := store.SetPluginIDTx(ctx, tx, instanceID, release.PluginID); err != nil {
			return err
		}
		return store.ActivateReleaseTx(ctx, tx, instanceID, release.ID)
	})
}

func persistActivatedReleaseFallback(ctx context.Context, store authoringInstanceStore, instanceID string, release plugininstances.Release) (bool, error) {
	if err := store.CreateRelease(ctx, release); err != nil {
		return false, err
	}
	if err := store.SetPluginID(ctx, instanceID, release.PluginID); err != nil {
		return true, err
	}
	if err := store.ActivateRelease(ctx, instanceID, release.ID); err != nil {
		return true, err
	}
	return true, nil
}

// Releases returns release history for a canvas after checking its durable
// metadata binding.
func (s *Service) Releases(ctx context.Context, canvasID string) ([]plugininstances.Release, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	_, instance, err := s.load(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	return store.ListReleases(ctx, instance.ID)
}

// PromotionPreview builds the human review projection for a task-scoped
// canvas. It does not mutate scope, grants, or release state.
func (s *Service) PromotionPreview(ctx context.Context, canvasID string) (*PromotionPreview, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	canvas, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	if canvas.ScopeKind != ScopeTask || canvas.ActiveReleaseID == "" || canvas.ActiveReleaseStatus != ValidationValid {
		return nil, fmt.Errorf("%w: canvas must have a valid task-scoped release", ErrInvalidCanvasState)
	}
	release, err := store.GetRelease(ctx, canvas.ActiveReleaseID)
	if err != nil {
		return nil, err
	}
	m, err := manifestFromRelease(release)
	if err != nil {
		return nil, err
	}
	return &PromotionPreview{
		Canvas:           canvas,
		SourceActorKind:  release.SourceActorKind,
		SourceUserID:     release.SourceUserID,
		SourceTaskID:     release.SourceTaskID,
		SourceSessionID:  release.SourceSessionID,
		Permissions:      ManifestPermissions(m),
		ActiveReleaseID:  release.ID,
		PermissionDigest: PermissionDigest(release),
		GrantGeneration:  canvas.GrantGeneration,
		CurrentScope:     ScopeTask,
		TargetScope:      ScopeWorkspace,
		Placement:        manifest.WebAppPlacementWorkspace,
	}, nil
}

// PromoteCanvas atomically approves the active declaration and changes the
// instance scope with the canvas metadata provenance. The durable store
// transaction keeps both lifecycle authorities consistent after a crash.
func (s *Service) PromoteCanvas(ctx context.Context, canvasID, userID string) (*Canvas, error) {
	return s.PromoteCanvasReviewed(ctx, canvasID, userID, "", "", 0)
}

// PromoteCanvasReviewed atomically applies a promotion only when the release
// and permission projection still match the human review response.
func (s *Service) PromoteCanvasReviewed(ctx context.Context, canvasID, userID, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: approving user is required", ErrInvalidCanvas)
	}
	preview, err := s.PromotionPreview(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	grants := grantsForManifest(preview.Permissions, userID, ScopeWorkspace)
	promotedAt := s.nowUTC()
	if err := s.promoteInstanceForReview(ctx, store, preview, canvasID, userID, grants, promotedAt, expectedReleaseID, expectedPermissionDigest, expectedGrantGeneration); err != nil {
		return nil, err
	}
	updated, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	publisher := s.publisher
	s.mu.Unlock()
	publishEvent(ctx, publisher, lifecycleEvent(EventPromoted, *updated))
	return updated, nil
}

func (s *Service) promoteInstanceForReview(ctx context.Context, store authoringInstanceStore, preview *PromotionPreview, canvasID, userID string, grants []plugininstances.Grant, promotedAt time.Time, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) error {
	reviewed := expectedReleaseID != "" || expectedPermissionDigest != "" || expectedGrantGeneration > 0
	transactional, isTransactional := store.(transactionalPluginInstanceStore)
	if isTransactional {
		err := transactional.WithTransaction(ctx, func(tx *sqlx.Tx) error {
			if err := promoteInstanceTx(ctx, tx, transactional, preview, userID, grants, reviewed, expectedReleaseID, expectedPermissionDigest, expectedGrantGeneration); err != nil {
				return err
			}
			return s.repo.PromoteTx(ctx, tx, canvasID, userID, promotedAt)
		})
		return err
	}
	if reviewed {
		return ErrStalePromotionReview
	}
	if err := store.PromoteScopeAndGrants(ctx, preview.Canvas.PluginInstanceID, preview.Canvas.WorkspaceID, userID, grants); err != nil {
		return err
	}
	if err := s.repo.Promote(ctx, canvasID, userID, promotedAt); err != nil {
		return errors.Join(ErrCanvasMetadataBroken, err)
	}
	return nil
}

func promoteInstanceTx(ctx context.Context, tx *sqlx.Tx, store transactionalPluginInstanceStore, preview *PromotionPreview, userID string, grants []plugininstances.Grant, reviewed bool, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) error {
	if reviewed {
		reviewedStore, ok := store.(reviewedPromotionStore)
		if !ok {
			return ErrStalePromotionReview
		}
		return reviewedStore.PromoteScopeAndGrantsReviewedTx(ctx, tx, preview.Canvas.PluginInstanceID, preview.Canvas.WorkspaceID, userID, grants, expectedReleaseID, expectedPermissionDigest, expectedGrantGeneration)
	}
	return store.PromoteScopeAndGrantsTx(ctx, tx, preview.Canvas.PluginInstanceID, preview.Canvas.WorkspaceID, userID, grants)
}

// ApproveRelease grants the requested permissions and activates one pending
// release without changing the canvas scope.
func (s *Service) ApproveRelease(ctx context.Context, canvasID, releaseID, userID string) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	canvas, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	if err := releaseMutationStateError(canvas.Status); err != nil {
		return nil, err
	}
	release, err := store.GetRelease(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	if release.InstanceID != canvas.PluginInstanceID {
		return nil, plugininstances.ErrInvalidRelease
	}
	m, err := manifestFromRelease(release)
	if err != nil {
		return nil, err
	}
	if err := store.ApproveRelease(ctx, canvas.PluginInstanceID, releaseID, userID, grantsForManifest(ManifestPermissions(m), userID, canvas.ScopeKind)); err != nil {
		return nil, err
	}
	if err := prunePublishedReleases(ctx, store, canvas.PluginInstanceID); err != nil {
		return nil, err
	}
	updated, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	publisher := s.publisher
	s.mu.Unlock()
	publishEvent(ctx, publisher, lifecycleEvent(EventReleaseActivated, *updated))
	return updated, nil
}

// RejectRelease keeps the active release untouched and records a safe
// rejection diagnostic on the pending release.
func (s *Service) RejectRelease(ctx context.Context, canvasID, releaseID string) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	canvas, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	if err := releaseMutationStateError(canvas.Status); err != nil {
		return nil, err
	}
	release, err := store.GetRelease(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	if release.InstanceID != canvas.PluginInstanceID || release.ValidationStatus != ValidationPendingPermission {
		return nil, plugininstances.ErrInvalidRelease
	}
	if rejection, ok := store.(releaseRejectionStore); ok {
		if err := rejection.RejectReleaseAndPrune(ctx, canvas.PluginInstanceID, releaseID); err != nil {
			return nil, err
		}
	} else {
		if err := store.SetReleaseValidation(ctx, releaseID, plugininstances.ValidationInvalid, "rejected_by_user"); err != nil {
			return nil, err
		}
		if err := prunePublishedReleases(ctx, store, canvas.PluginInstanceID); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, canvasID)
}

// RollbackRelease selects a retained valid release and rechecks its required
// permissions against current grants before moving the active pointer.
func (s *Service) RollbackRelease(ctx context.Context, canvasID, releaseID string) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	store, ok := s.instances.(authoringInstanceStore)
	if !ok {
		return nil, ErrCanvasNotConfigured
	}
	canvas, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	if err := releaseMutationStateError(canvas.Status); err != nil {
		return nil, err
	}
	instance, err := store.Get(ctx, canvas.PluginInstanceID)
	if err != nil {
		return nil, err
	}
	releases, err := store.ListReleases(ctx, instance.ID)
	if err != nil {
		return nil, err
	}
	release, err := selectRollbackRelease(ctx, store, instance, releases, releaseID)
	if err != nil {
		return nil, err
	}
	m, err := manifestFromRelease(release)
	if err != nil {
		return nil, err
	}
	grants, err := store.ListGrants(ctx, instance.ID)
	if err != nil {
		return nil, err
	}
	if !permissionsFit(ManifestPermissions(m), instance.ScopeKind, grants) {
		return nil, fmt.Errorf("%w: rollback requires permission review", plugininstances.ErrInvalidRelease)
	}
	if err := store.ActivateRelease(ctx, instance.ID, release.ID); err != nil {
		return nil, err
	}
	if err := prunePublishedReleases(ctx, store, instance.ID); err != nil {
		return nil, err
	}
	updated, err := s.Get(ctx, canvasID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	publisher := s.publisher
	s.mu.Unlock()
	publishEvent(ctx, publisher, lifecycleEvent(EventReleaseActivated, *updated))
	return updated, nil
}

func selectRollbackRelease(ctx context.Context, store authoringInstanceStore, instance plugininstances.Instance, releases []plugininstances.Release, releaseID string) (plugininstances.Release, error) {
	if releaseID == "" {
		for _, candidate := range releases {
			if candidate.ID != instance.ActiveReleaseID && candidate.ValidationStatus == ValidationValid {
				releaseID = candidate.ID
				break
			}
		}
	}
	if releaseID == "" {
		return plugininstances.Release{}, fmt.Errorf("%w: no retained valid release", plugininstances.ErrInvalidRelease)
	}
	release, err := store.GetRelease(ctx, releaseID)
	if err != nil {
		return plugininstances.Release{}, err
	}
	if release.InstanceID != instance.ID || release.ID == instance.ActiveReleaseID || release.ValidationStatus != ValidationValid {
		return plugininstances.Release{}, plugininstances.ErrInvalidRelease
	}
	return release, nil
}

func prunePublishedReleases(ctx context.Context, store authoringInstanceStore, instanceID string) error {
	retention, ok := store.(releaseRetentionStore)
	if !ok {
		return nil
	}
	return retention.PruneReleases(ctx, instanceID)
}

func releaseMutationStateError(status string) error {
	if status == StatusArchived || status == StatusDisabled {
		return fmt.Errorf("%w: canvas is %s", ErrInvalidCanvasState, status)
	}
	return nil
}

// ManifestPermissions converts the manifest into the review/runtime shape.
func ManifestPermissions(m *manifest.Manifest) PermissionSummary {
	if m == nil {
		return PermissionSummary{}
	}
	origins := nonEmptyStrings([]string{m.BaseURL})
	seenOrigins := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		seenOrigins[origin] = struct{}{}
	}
	for _, app := range m.UI.WebApps {
		normalized, err := webapp.NormalizeNetworkOrigins(app.NetworkOrigins)
		if err != nil {
			continue
		}
		for _, origin := range normalized {
			if _, exists := seenOrigins[origin]; exists {
				continue
			}
			seenOrigins[origin] = struct{}{}
			origins = append(origins, origin)
		}
	}
	return PermissionSummary{
		Reads:           append([]string(nil), m.Capabilities.APIRead...),
		Writes:          append([]string(nil), m.Capabilities.APIWrite...),
		Events:          append([]string(nil), m.Capabilities.Events...),
		SharedState:     m.Capabilities.State,
		ExternalOrigins: origins,
	}
}

func manifestFromRelease(release plugininstances.Release) (*manifest.Manifest, error) {
	var m manifest.Manifest
	if err := json.Unmarshal(release.ManifestJSON, &m); err != nil {
		return nil, fmt.Errorf("parse release manifest: %w", err)
	}
	return &m, nil
}

// ReleasePermissionSummary returns the review-safe declaration persisted with
// a release. Older rows used a flat permission-key array, which remains
// readable so a release review never falls back to an empty declaration.
func ReleasePermissionSummary(release plugininstances.Release) PermissionSummary {
	var summary PermissionSummary
	if err := json.Unmarshal(release.DeclaredPermissionsJSON, &summary); err == nil {
		return summary
	}
	var legacy []string
	if err := json.Unmarshal(release.DeclaredPermissionsJSON, &legacy); err != nil {
		return summary
	}
	for _, permission := range legacy {
		parts := strings.SplitN(permission, ":", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case permissionKindAPIRead:
			summary.Reads = append(summary.Reads, parts[1])
		case permissionKindAPIWrite:
			summary.Writes = append(summary.Writes, parts[1])
		case permissionKindEvents:
			summary.Events = append(summary.Events, parts[1])
		case permissionKindNetwork:
			summary.ExternalOrigins = append(summary.ExternalOrigins, parts[1])
		case permissionKindState:
			summary.SharedState = true
		}
	}
	return summary
}

// PermissionDigest returns the digest of the exact persisted declaration. It
// is safe to show to a reviewer and lets confirmation reject a changed
// declaration without exposing its manifest or source package.
func PermissionDigest(release plugininstances.Release) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(string(release.DeclaredPermissionsJSON))))
	return fmt.Sprintf("%x", digest[:])
}

// MissingPermissionKeys reports declaration keys not covered by the current
// grants at the release scope. The result is sorted for stable API output.
func MissingPermissionKeys(summary PermissionSummary, scope string, grants []plugininstances.Grant) []string {
	missing := make([]string, 0)
	for _, permission := range permissionKeys(summary) {
		if !grantCovers(permission, scope, grants) {
			missing = append(missing, permission)
		}
	}
	sort.Strings(missing)
	return missing
}

func permissionKeys(summary PermissionSummary) []string {
	keys := make([]string, 0, len(summary.Reads)+len(summary.Writes)+len(summary.Events)+len(summary.ExternalOrigins)+1)
	for _, value := range summary.Reads {
		keys = append(keys, permissionKindAPIRead+":"+value)
	}
	for _, value := range summary.Writes {
		keys = append(keys, permissionKindAPIWrite+":"+value)
	}
	for _, value := range summary.Events {
		keys = append(keys, permissionKindEvents+":"+value)
	}
	for _, value := range summary.ExternalOrigins {
		keys = append(keys, permissionKindNetwork+":"+value)
	}
	if summary.SharedState {
		keys = append(keys, permissionKindState)
	}
	return keys
}

func permissionsFit(summary PermissionSummary, scope string, grants []plugininstances.Grant) bool {
	if len(permissionKeys(summary)) == 0 {
		return true
	}
	for _, required := range permissionKeys(summary) {
		if !grantCovers(required, scope, grants) {
			return false
		}
	}
	return true
}

func grantCovers(permission, scope string, grants []plugininstances.Grant) bool {
	parts := strings.SplitN(permission, ":", 2)
	kind, resource := parts[0], ""
	if len(parts) == 2 {
		resource = parts[1]
	}
	for _, grant := range grants {
		if kind == permissionKindNetwork {
			if grant.PermissionKind == kind && grant.NetworkOrigin == resource && grantScopeCovers(grant.ScopeCeiling, scope) {
				return true
			}
			continue
		}
		if grant.PermissionKind == kind && grant.Resource == resource && grantScopeCovers(grant.ScopeCeiling, scope) {
			return true
		}
	}
	return false
}

func grantScopeCovers(ceiling, scope string) bool {
	return ceiling == plugininstances.ScopeInstance || ceiling == scope
}

func grantsForManifest(summary PermissionSummary, userID, scope string) []plugininstances.Grant {
	grantScope := scope
	if grantScope == "" {
		grantScope = ScopeWorkspace
	}
	now := time.Now().UTC()
	grants := make([]plugininstances.Grant, 0, len(permissionKeys(summary)))
	for _, permission := range permissionKeys(summary) {
		parts := strings.SplitN(permission, ":", 2)
		resource := ""
		networkOrigin := ""
		if len(parts) == 2 {
			resource = parts[1]
			if parts[0] == permissionKindNetwork {
				networkOrigin = resource
				resource = ""
			}
		}
		grants = append(grants, plugininstances.Grant{
			PermissionKind: parts[0], Resource: resource, NetworkOrigin: networkOrigin, ScopeCeiling: grantScope,
			ApprovedBy: userID, ApprovedAt: now,
		})
	}
	return grants
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" && !containsString(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
