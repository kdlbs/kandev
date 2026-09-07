package canvas

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
)

// Service coordinates canvas metadata with its plugin instance. The mutex
// covers admission and lifecycle mutations so a create, restore, or cleanup
// cannot observe a partially completed operation in this process. The plugin
// instance store remains the durable admission authority.
type Service struct {
	repo      *Repository
	instances PluginInstanceStore

	mu           sync.Mutex
	clock        func() time.Time
	publisher    EventPublisher
	stateCleanup func(context.Context, string) error
}

// NewService constructs a canvas lifecycle service. The instance store is
// normally *instances.Store from internal/plugins/instances.
func NewService(repo *Repository, instanceStore PluginInstanceStore) *Service {
	return &Service{repo: repo, instances: instanceStore, clock: time.Now}
}

// SetEventPublisher attaches the content-free lifecycle notification sink.
// Wire it before serving requests. Publishing occurs after the database
// mutation has committed and never determines mutation success.
func (s *Service) SetEventPublisher(publisher EventPublisher) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.publisher = publisher
	s.mu.Unlock()
}

// SetInstanceStateCleanup wires the instance-state owner into canvas removal.
// The callback is optional so the lifecycle service remains usable with the
// narrow in-memory stores used by tests and migrations.
func (s *Service) SetInstanceStateCleanup(cleanup func(context.Context, string) error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stateCleanup = cleanup
	s.mu.Unlock()
}

// Create creates a pending canvas and its task- or workspace-scoped plugin
// instance. A task ID creates a task canvas; an empty task ID is reserved for
// trusted internal callers creating a workspace-scoped canvas.
func (s *Service) Create(ctx context.Context, request CreateCanvasRequest) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	request, err := normalizeCreateRequest(request)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	canvas, event, err := s.createLocked(ctx, request)
	publisher := s.publisher
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	publishEvent(ctx, publisher, event)
	return &canvas, nil
}

// CreateCanvas is the descriptive alias used by HTTP and MCP adapters.
func (s *Service) CreateCanvas(ctx context.Context, request CreateCanvasRequest) (*Canvas, error) {
	return s.Create(ctx, request)
}

func (s *Service) createLocked(ctx context.Context, request CreateCanvasRequest) (Canvas, LifecycleEvent, error) {
	now := s.nowUTC()
	canvasID := uuid.NewString()
	instanceID := uuid.NewString()
	scopeKind := ScopeWorkspace
	if request.TaskID != "" {
		scopeKind = ScopeTask
	}
	metadata := CanvasMetadata{
		ID:                 canvasID,
		PluginInstanceID:   instanceID,
		WorkspaceID:        request.WorkspaceID,
		TaskID:             request.TaskID,
		OriginTaskID:       request.OriginTaskID,
		Title:              request.Title,
		CreatedBySessionID: request.CreatedBySessionID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	instance := plugininstances.Instance{
		ID:          instanceID,
		PluginID:    request.PluginID,
		SourceKind:  plugininstances.SourceLocalCanvas,
		ScopeKind:   scopeKind,
		WorkspaceID: request.WorkspaceID,
		TaskID:      request.TaskID,
		Status:      plugininstances.StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.createAuthority(ctx, instance, metadata); err != nil {
		return Canvas{}, LifecycleEvent{}, err
	}
	canvas := canvasFromMetadataInstance(metadata, instance)
	return canvas, lifecycleEvent(EventCreated, canvas), nil
}

func (s *Service) createAuthority(ctx context.Context, instance plugininstances.Instance, metadata CanvasMetadata) error {
	transactional, ok := s.instances.(transactionalPluginInstanceStore)
	if ok {
		return transactional.WithTransaction(ctx, func(tx *sqlx.Tx) error {
			if err := transactional.CreateTx(ctx, tx, instance); err != nil {
				return err
			}
			return s.repo.CreateTx(ctx, tx, metadata)
		})
	}
	if err := s.instances.Create(ctx, instance); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, metadata); err != nil {
		cleanupErr := s.compensateCreate(instance.ID)
		if cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("remove admitted plugin instance: %w", cleanupErr))
		}
		return err
	}
	return nil
}

// Get returns one non-removed canvas and reports an unavailable active release
// in the projection without attempting to execute or remove that release.
func (s *Service) Get(ctx context.Context, id string) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	metadata, instance, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if instance.Status == StatusRemoved {
		return nil, ErrCanvasNotFound
	}
	canvas, err := s.buildCanvas(ctx, metadata, instance)
	if err != nil {
		return nil, err
	}
	return &canvas, nil
}

// GetCanvas is the descriptive alias used by API adapters.
func (s *Service) GetCanvas(ctx context.Context, id string) (*Canvas, error) {
	return s.Get(ctx, id)
}

// ListTaskCanvases lists task-scoped canvases. Removed canvases are never
// returned; archived canvases are returned only when includeArchived is true.
func (s *Service) ListTaskCanvases(ctx context.Context, taskID string, includeArchived bool) ([]Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	metadata, err := s.repo.ListByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.listMetadata(ctx, metadata, includeArchived, isTaskCanvas)
}

// ListWorkspaceCanvases lists promoted or internally-created workspace-scoped
// canvases. Task-only canvases are not included.
func (s *Service) ListWorkspaceCanvases(ctx context.Context, workspaceID string, includeArchived bool) ([]Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	metadata, err := s.repo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.listMetadata(ctx, metadata, includeArchived, isWorkspaceCanvas)
}

// ListForTask combines task canvases with workspace canvases applicable to
// the task's workspace.
func (s *Service) ListForTask(ctx context.Context, workspaceID, taskID string, includeArchived bool) ([]Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, ErrInvalidCanvas
	}
	taskMetadata, err := s.repo.ListByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	workspaceMetadata, err := s.repo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	workspaceOnly := make([]CanvasMetadata, 0, len(workspaceMetadata))
	for _, item := range workspaceMetadata {
		if item.TaskID == "" {
			workspaceOnly = append(workspaceOnly, item)
		}
	}
	metadata := make([]CanvasMetadata, 0, len(taskMetadata)+len(workspaceOnly))
	metadata = append(metadata, taskMetadata...)
	metadata = append(metadata, workspaceOnly...)
	return s.listMetadata(ctx, metadata, includeArchived, func(item CanvasMetadata, instance plugininstances.Instance) bool {
		if item.WorkspaceID != workspaceID {
			return false
		}
		return (isTaskCanvas(item, instance) && item.TaskID == taskID) || isWorkspaceCanvas(item, instance)
	})
}

// ListCanvasesForTask is an alias for ListForTask.
func (s *Service) ListCanvasesForTask(ctx context.Context, workspaceID, taskID string, includeArchived bool) ([]Canvas, error) {
	return s.ListForTask(ctx, workspaceID, taskID, includeArchived)
}

// Archive changes a canvas to the archived instance state. Archived canvases
// remain admitted and therefore continue to count against both limits.
func (s *Service) Archive(ctx context.Context, id string) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	canvas, event, changed, err := s.archiveLocked(ctx, id)
	publisher := s.publisher
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if changed {
		publishEvent(ctx, publisher, event)
	}
	return &canvas, nil
}

// ArchiveCanvas is the descriptive alias used by API adapters.
func (s *Service) ArchiveCanvas(ctx context.Context, id string) (*Canvas, error) {
	return s.Archive(ctx, id)
}

func (s *Service) archiveLocked(ctx context.Context, id string) (Canvas, LifecycleEvent, bool, error) {
	metadata, instance, err := s.load(ctx, id)
	if err != nil {
		return Canvas{}, LifecycleEvent{}, false, err
	}
	if instance.Status == StatusRemoved {
		return Canvas{}, LifecycleEvent{}, false, ErrCanvasNotFound
	}
	if instance.Status == StatusArchived {
		canvas, err := s.buildCanvas(ctx, metadata, instance)
		return canvas, LifecycleEvent{}, false, err
	}
	if err := s.instances.Archive(ctx, instance.ID); err != nil {
		return Canvas{}, LifecycleEvent{}, false, err
	}
	instance, err = s.instances.Get(ctx, instance.ID)
	if err != nil {
		return Canvas{}, LifecycleEvent{}, false, err
	}
	canvas, err := s.buildCanvas(ctx, metadata, instance)
	if err != nil {
		return Canvas{}, LifecycleEvent{}, false, err
	}
	return canvas, lifecycleEvent(EventArchived, canvas), true, nil
}

// Restore re-admits an archived canvas through the instance store's atomic
// limit check. A full task or workspace returns the stable admission error.
func (s *Service) Restore(ctx context.Context, id string) (*Canvas, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	canvas, event, err := s.restoreLocked(ctx, id)
	publisher := s.publisher
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	publishEvent(ctx, publisher, event)
	return &canvas, nil
}

// RestoreCanvas is the descriptive alias used by API adapters.
func (s *Service) RestoreCanvas(ctx context.Context, id string) (*Canvas, error) {
	return s.Restore(ctx, id)
}

func (s *Service) restoreLocked(ctx context.Context, id string) (Canvas, LifecycleEvent, error) {
	metadata, instance, err := s.load(ctx, id)
	if err != nil {
		return Canvas{}, LifecycleEvent{}, err
	}
	if instance.Status == StatusRemoved {
		return Canvas{}, LifecycleEvent{}, ErrCanvasNotFound
	}
	if instance.Status != StatusArchived {
		return Canvas{}, LifecycleEvent{}, fmt.Errorf("%w: canvas is %s", ErrInvalidCanvasState, instance.Status)
	}
	if err := s.instances.Restore(ctx, instance.ID); err != nil {
		return Canvas{}, LifecycleEvent{}, err
	}
	instance, err = s.instances.Get(ctx, instance.ID)
	if err != nil {
		return Canvas{}, LifecycleEvent{}, err
	}
	canvas, err := s.buildCanvas(ctx, metadata, instance)
	if err != nil {
		return Canvas{}, LifecycleEvent{}, err
	}
	return canvas, lifecycleEvent(EventRestored, canvas), nil
}

// Remove first asks the plugin instance store to record cleanup jobs and
// remove live instance authority, then deletes canvas metadata. This ordering
// makes artifact cleanup durable if the process stops between both commits.
func (s *Service) Remove(ctx context.Context, id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	s.mu.Lock()
	event, err := s.removeLocked(ctx, id)
	publisher := s.publisher
	s.mu.Unlock()
	if err != nil {
		return err
	}
	publishEvent(ctx, publisher, event)
	return nil
}

// RemoveCanvas is the descriptive alias used by API adapters.
func (s *Service) RemoveCanvas(ctx context.Context, id string) error {
	return s.Remove(ctx, id)
}

func (s *Service) removeLocked(ctx context.Context, id string) (LifecycleEvent, error) {
	metadata, instance, err := s.load(ctx, id)
	if err != nil {
		return LifecycleEvent{}, err
	}
	if err := s.removeAuthority(ctx, metadata, instance); err != nil {
		return LifecycleEvent{}, err
	}
	instance.Status = StatusRemoved
	instance.ActiveReleaseID = ""
	if s.stateCleanup != nil {
		if err := s.stateCleanup(ctx, instance.ID); err != nil {
			return LifecycleEvent{}, err
		}
	}
	canvas := canvasFromMetadataInstance(metadata, instance)
	return lifecycleEvent(EventRemoved, canvas), nil
}

func (s *Service) removeAuthority(ctx context.Context, metadata CanvasMetadata, instance plugininstances.Instance) error {
	if instance.Status == StatusRemoved {
		err := s.repo.Delete(ctx, metadata.ID)
		if errors.Is(err, ErrCanvasNotFound) {
			return nil
		}
		return err
	}
	transactional, ok := s.instances.(transactionalPluginInstanceStore)
	if ok {
		return transactional.WithTransaction(ctx, func(tx *sqlx.Tx) error {
			if err := transactional.RemoveInstanceTx(ctx, tx, instance.ID); err != nil {
				return err
			}
			return s.repo.DeleteTx(ctx, tx, metadata.ID)
		})
	}
	if err := s.instances.RemoveInstance(ctx, instance.ID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, metadata.ID)
}

// CleanupTask removes all current task-scoped canvases. Promotion clears the
// task_id in metadata, so promoted canvases are intentionally untouched.
func (s *Service) CleanupTask(ctx context.Context, taskID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	if strings.TrimSpace(taskID) == "" {
		return ErrInvalidCanvas
	}
	s.mu.Lock()
	metadata, err := s.repo.ListByTask(ctx, taskID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	events, err := s.cleanupMetadataLocked(ctx, metadata)
	if err == nil {
		err = s.repo.ClearOriginTask(ctx, taskID)
	}
	publisher := s.publisher
	s.mu.Unlock()
	for _, event := range events {
		publishEvent(ctx, publisher, event)
	}
	return err
}

// CleanupTaskCanvases is an explicit alias for task-service wiring.
func (s *Service) CleanupTaskCanvases(ctx context.Context, taskID string) error {
	return s.CleanupTask(ctx, taskID)
}

// CleanupWorkspace removes every canvas in a workspace, including archived
// and task-scoped canvases. Each plugin instance removal inventories its
// retained release artifacts before its metadata is deleted.
func (s *Service) CleanupWorkspace(ctx context.Context, workspaceID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	if strings.TrimSpace(workspaceID) == "" {
		return ErrInvalidCanvas
	}
	s.mu.Lock()
	metadata, err := s.repo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	events, err := s.cleanupMetadataLocked(ctx, metadata)
	publisher := s.publisher
	s.mu.Unlock()
	for _, event := range events {
		publishEvent(ctx, publisher, event)
	}
	return err
}

// CleanupWorkspaceCanvases is an explicit alias for workspace-service wiring.
func (s *Service) CleanupWorkspaceCanvases(ctx context.Context, workspaceID string) error {
	return s.CleanupWorkspace(ctx, workspaceID)
}

// Reconcile repairs lifecycle rows left across a process boundary. Canvas
// creation and removal are transactional for the durable store, but this
// pass also handles databases written by an older binary or a crash during
// the compatibility fallback. It never executes a release.
func (s *Service) Reconcile(ctx context.Context) error {
	if err := s.ready(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, err := s.repo.ListAll(ctx)
	if err != nil {
		return err
	}
	knownInstances, errs := s.reconcileMetadata(ctx, metadata)
	inventory, ok := s.instances.(interface {
		ListBySource(context.Context, string, bool) ([]plugininstances.Instance, error)
	})
	if !ok {
		return errors.Join(errs...)
	}
	instances, err := inventory.ListBySource(ctx, plugininstances.SourceLocalCanvas, true)
	if err != nil {
		errs = append(errs, fmt.Errorf("list canvas instances: %w", err))
		return errors.Join(errs...)
	}
	errs = append(errs, s.reconcileOrphanInstances(ctx, instances, knownInstances)...)
	return errors.Join(errs...)
}

func (s *Service) reconcileMetadata(ctx context.Context, metadata []CanvasMetadata) (map[string]struct{}, []error) {
	knownInstances := make(map[string]struct{}, len(metadata))
	for _, item := range metadata {
		knownInstances[item.PluginInstanceID] = struct{}{}
	}
	instancesByID, err := s.loadMetadataInstances(ctx, metadata)
	if err != nil {
		return knownInstances, []error{fmt.Errorf("inspect canvas instances: %w", err)}
	}
	var errs []error
	for _, item := range metadata {
		instance, exists := instancesByID[item.PluginInstanceID]
		if exists && instance.Status != plugininstances.StatusRemoved {
			continue
		}
		if _, err := s.removeMetadataLocked(ctx, item); err != nil {
			errs = append(errs, fmt.Errorf("remove orphan canvas metadata %s: %w", item.ID, err))
		}
	}
	return knownInstances, errs
}

func (s *Service) reconcileOrphanInstances(ctx context.Context, instances []plugininstances.Instance, known map[string]struct{}) []error {
	var errs []error
	for _, instance := range instances {
		if _, exists := known[instance.ID]; exists {
			continue
		}
		if instance.Status != plugininstances.StatusRemoved {
			if err := s.instances.RemoveInstance(ctx, instance.ID); err != nil && !errors.Is(err, plugininstances.ErrNotFound) {
				errs = append(errs, fmt.Errorf("remove orphan canvas instance %s: %w", instance.ID, err))
				continue
			}
		}
		if s.stateCleanup != nil {
			if err := s.stateCleanup(ctx, instance.ID); err != nil {
				errs = append(errs, fmt.Errorf("remove orphan canvas state %s: %w", instance.ID, err))
			}
		}
	}
	return errs
}

func (s *Service) cleanupMetadataLocked(ctx context.Context, metadata []CanvasMetadata) ([]LifecycleEvent, error) {
	var errs []error
	events := make([]LifecycleEvent, 0, len(metadata))
	for _, item := range metadata {
		event, err := s.removeMetadataLocked(ctx, item)
		if err == nil {
			events = append(events, event)
			continue
		}
		if !errors.Is(err, ErrCanvasNotFound) {
			errs = append(errs, fmt.Errorf("remove canvas %s: %w", item.ID, err))
		}
	}
	return events, errors.Join(errs...)
}

func (s *Service) load(ctx context.Context, id string) (CanvasMetadata, plugininstances.Instance, error) {
	metadata, err := s.repo.Get(ctx, id)
	if err != nil {
		return CanvasMetadata{}, plugininstances.Instance{}, err
	}
	instance, err := s.instances.Get(ctx, metadata.PluginInstanceID)
	if errors.Is(err, plugininstances.ErrNotFound) {
		return CanvasMetadata{}, plugininstances.Instance{}, ErrCanvasNotFound
	}
	if err != nil {
		return CanvasMetadata{}, plugininstances.Instance{}, err
	}
	return metadata, instance, nil
}

func (s *Service) removeMetadataLocked(ctx context.Context, metadata CanvasMetadata) (LifecycleEvent, error) {
	instance, err := s.instances.Get(ctx, metadata.PluginInstanceID)
	if errors.Is(err, plugininstances.ErrNotFound) {
		// A crash can happen after instance removal commits but before the
		// metadata transaction commits. There is no remaining instance
		// authority to remove, so clear its state and finish metadata cleanup.
		if s.stateCleanup != nil {
			if err := s.stateCleanup(ctx, metadata.PluginInstanceID); err != nil {
				return LifecycleEvent{}, err
			}
		}
		if err := s.repo.Delete(ctx, metadata.ID); err != nil && !errors.Is(err, ErrCanvasNotFound) {
			return LifecycleEvent{}, err
		}
		instance = plugininstances.Instance{
			ID: metadata.PluginInstanceID, PluginID: CanvasPluginID,
			ScopeKind: ScopeWorkspace, WorkspaceID: metadata.WorkspaceID,
			Status: StatusRemoved,
		}
		canvas := canvasFromMetadataInstance(metadata, instance)
		return lifecycleEvent(EventRemoved, canvas), nil
	}
	if err != nil {
		return LifecycleEvent{}, err
	}
	if err := s.removeAuthority(ctx, metadata, instance); err != nil {
		return LifecycleEvent{}, err
	}
	instance.Status = StatusRemoved
	instance.ActiveReleaseID = ""
	if s.stateCleanup != nil {
		if err := s.stateCleanup(ctx, instance.ID); err != nil {
			return LifecycleEvent{}, err
		}
	}
	canvas := canvasFromMetadataInstance(metadata, instance)
	return lifecycleEvent(EventRemoved, canvas), nil
}

func (s *Service) listMetadata(
	ctx context.Context,
	metadata []CanvasMetadata,
	includeArchived bool,
	allowed func(CanvasMetadata, plugininstances.Instance) bool,
) ([]Canvas, error) {
	instancesByID, err := s.loadMetadataInstances(ctx, metadata)
	if err != nil {
		return nil, err
	}
	canvases := make([]Canvas, 0, len(metadata))
	for _, item := range metadata {
		instance, exists := instancesByID[item.PluginInstanceID]
		if !exists || instance.Status == StatusRemoved {
			continue
		}
		if !includeArchived && instance.Status == StatusArchived {
			continue
		}
		if allowed != nil && !allowed(item, instance) {
			continue
		}
		canvas, err := s.buildCanvas(ctx, item, instance)
		if err != nil {
			return nil, err
		}
		canvases = append(canvases, canvas)
	}
	sort.SliceStable(canvases, func(i, j int) bool {
		if canvases[i].CreatedAt.Equal(canvases[j].CreatedAt) {
			return canvases[i].ID < canvases[j].ID
		}
		return canvases[i].CreatedAt.Before(canvases[j].CreatedAt)
	})
	return canvases, nil
}

type bulkPluginInstanceStore interface {
	GetMany(context.Context, []string) ([]plugininstances.Instance, error)
}

func (s *Service) loadMetadataInstances(ctx context.Context, metadata []CanvasMetadata) (map[string]plugininstances.Instance, error) {
	ids := make([]string, 0, len(metadata))
	seen := make(map[string]struct{}, len(metadata))
	for _, item := range metadata {
		if _, exists := seen[item.PluginInstanceID]; exists {
			continue
		}
		seen[item.PluginInstanceID] = struct{}{}
		ids = append(ids, item.PluginInstanceID)
	}
	result := make(map[string]plugininstances.Instance, len(ids))
	if bulk, ok := s.instances.(bulkPluginInstanceStore); ok {
		instances, err := bulk.GetMany(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, instance := range instances {
			result[instance.ID] = instance
		}
		return result, nil
	}
	for _, id := range ids {
		instance, err := s.instances.Get(ctx, id)
		if errors.Is(err, plugininstances.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result[instance.ID] = instance
	}
	return result, nil
}

func (s *Service) buildCanvas(ctx context.Context, metadata CanvasMetadata, instance plugininstances.Instance) (Canvas, error) {
	if err := validateInstanceBinding(metadata, instance); err != nil {
		return Canvas{}, err
	}
	canvas := canvasFromMetadataInstance(metadata, instance)
	grants, err := s.listGrants(ctx, instance.ID)
	if err != nil {
		return Canvas{}, err
	}
	if instance.ActiveReleaseID == "" {
		return s.addPendingRelease(ctx, &canvas, instance.ID, instance.ScopeKind, grants)
	}
	release, err := s.instances.GetRelease(ctx, instance.ActiveReleaseID)
	if errors.Is(err, plugininstances.ErrNotFound) {
		canvas.ActiveReleaseStatus = ValidationUnavailable
		canvas.ActiveReleaseError = "active_release_missing"
		canvas.ActiveRelease = &ReleaseMetadata{ID: instance.ActiveReleaseID, ValidationStatus: ValidationUnavailable, ValidationError: canvas.ActiveReleaseError}
		return canvas, nil
	}
	if err != nil {
		return Canvas{}, err
	}
	if release.InstanceID != instance.ID {
		return Canvas{}, fmt.Errorf("%w: active release %s belongs to another instance", ErrCanvasMetadataBroken, release.ID)
	}
	canvas.ActiveReleaseStatus = release.ValidationStatus
	canvas.ActiveReleaseError = release.ValidationError
	canvas.EffectiveGrants = effectiveGrantProjection(instance, ReleasePermissionSummary(release), grants)
	canvas.ActiveRelease = releaseMetadata(release, instance.ScopeKind, grants)
	return canvas, nil
}

type releaseLister interface {
	ListReleases(context.Context, string) ([]plugininstances.Release, error)
}

func (s *Service) addPendingRelease(ctx context.Context, canvas *Canvas, instanceID, scope string, grants []plugininstances.Grant) (Canvas, error) {
	lister, ok := s.instances.(releaseLister)
	if !ok {
		return *canvas, nil
	}
	releases, err := lister.ListReleases(ctx, instanceID)
	if err != nil {
		return Canvas{}, err
	}
	for _, release := range releases {
		if release.ValidationStatus != ValidationPendingPermission {
			continue
		}
		canvas.PendingRelease = releaseMetadata(release, scope, grants)
		break
	}
	return *canvas, nil
}

type grantLister interface {
	ListGrants(context.Context, string) ([]plugininstances.Grant, error)
}

func (s *Service) listGrants(ctx context.Context, instanceID string) ([]plugininstances.Grant, error) {
	lister, ok := s.instances.(grantLister)
	if !ok {
		return nil, nil
	}
	return lister.ListGrants(ctx, instanceID)
}

func (s *Service) compensateCreate(instanceID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 5*time.Second)
	defer cancel()
	return s.instances.RemoveInstance(cleanupCtx, instanceID)
}

func (s *Service) ready() error {
	if s == nil || s.repo == nil || s.instances == nil {
		return ErrCanvasNotConfigured
	}
	return nil
}

func (s *Service) nowUTC() time.Time {
	if s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock().UTC()
}

func normalizeCreateRequest(request CreateCanvasRequest) (CreateCanvasRequest, error) {
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.OriginTaskID = strings.TrimSpace(request.OriginTaskID)
	request.CreatedBySessionID = strings.TrimSpace(request.CreatedBySessionID)
	request.PluginID = strings.TrimSpace(request.PluginID)
	request.Title = strings.TrimSpace(request.Title)
	if request.PluginID == "" {
		request.PluginID = CanvasPluginID
	}
	if request.WorkspaceID == "" {
		return CreateCanvasRequest{}, fmt.Errorf("%w: workspace_id is required", ErrInvalidCanvas)
	}
	if request.Title == "" || len([]rune(request.Title)) > MaxTitleLength {
		return CreateCanvasRequest{}, fmt.Errorf("%w: title must contain 1-%d characters", ErrInvalidCanvas, MaxTitleLength)
	}
	if request.TaskID != "" && request.OriginTaskID == "" {
		request.OriginTaskID = request.TaskID
	}
	return request, nil
}

func validateInstanceBinding(metadata CanvasMetadata, instance plugininstances.Instance) error {
	if metadata.PluginInstanceID != instance.ID || metadata.WorkspaceID != instance.WorkspaceID {
		return ErrCanvasMetadataBroken
	}
	if metadata.TaskID == "" {
		if instance.ScopeKind != ScopeWorkspace || instance.TaskID != "" {
			return ErrCanvasMetadataBroken
		}
		return nil
	}
	if instance.ScopeKind != ScopeTask || instance.TaskID != metadata.TaskID {
		return ErrCanvasMetadataBroken
	}
	return nil
}

func canvasFromMetadataInstance(metadata CanvasMetadata, instance plugininstances.Instance) Canvas {
	updatedAt := metadata.UpdatedAt
	if instance.UpdatedAt.After(updatedAt) {
		updatedAt = instance.UpdatedAt
	}
	return Canvas{
		ID:                 metadata.ID,
		PluginInstanceID:   instance.ID,
		PluginID:           instance.PluginID,
		WorkspaceID:        metadata.WorkspaceID,
		TaskID:             metadata.TaskID,
		OriginTaskID:       metadata.OriginTaskID,
		ScopeKind:          instance.ScopeKind,
		Title:              metadata.Title,
		CreatedBySessionID: metadata.CreatedBySessionID,
		PromotedByUserID:   metadata.PromotedByUserID,
		PromotedAt:         metadata.PromotedAt,
		Status:             instance.Status,
		ActiveReleaseID:    instance.ActiveReleaseID,
		GrantGeneration:    instance.GrantGeneration,
		CreatedAt:          metadata.CreatedAt,
		UpdatedAt:          updatedAt,
	}
}

func isTaskCanvas(_ CanvasMetadata, instance plugininstances.Instance) bool {
	return instance.ScopeKind == ScopeTask
}

func isWorkspaceCanvas(metadata CanvasMetadata, instance plugininstances.Instance) bool {
	return metadata.TaskID == "" && instance.ScopeKind == ScopeWorkspace
}

func lifecycleEvent(eventType string, canvas Canvas) LifecycleEvent {
	return LifecycleEvent{
		Type:                eventType,
		CanvasID:            canvas.ID,
		PluginInstanceID:    canvas.PluginInstanceID,
		WorkspaceID:         canvas.WorkspaceID,
		TaskID:              canvas.TaskID,
		ScopeKind:           canvas.ScopeKind,
		Status:              canvas.Status,
		ActiveReleaseID:     canvas.ActiveReleaseID,
		ActiveReleaseStatus: canvas.ActiveReleaseStatus,
	}
}

func publishEvent(ctx context.Context, publisher EventPublisher, event LifecycleEvent) {
	if publisher != nil {
		publisher(ctx, event)
	}
}
