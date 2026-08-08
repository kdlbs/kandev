// Package tempartifacts tracks exact temporary directories created by the
// Kandev service. A name prefix alone is never sufficient ownership proof.
package tempartifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/system/storage"
)

const (
	MarkerName               = ".kandev-temp-artifact.json"
	registryRunIDMetadataKey = "_kandev_registry_run_id"
)

type Store interface {
	CreateTemporaryArtifact(context.Context, *storage.TemporaryArtifact) error
	GetTemporaryArtifact(context.Context, string) (storage.TemporaryArtifact, error)
	ListTemporaryArtifacts(context.Context) ([]storage.TemporaryArtifact, error)
	HeartbeatTemporaryArtifact(context.Context, string, time.Time) error
	TransitionTemporaryArtifact(context.Context, string, storage.TemporaryArtifactState, string, time.Time) error
}

type Config struct {
	Store    Store
	TempRoot string
	OwnerPID int64
	RunID    string
	Now      func() time.Time
	NewID    func() string
	NewToken func() string
}

type Registry struct {
	store    Store
	tempRoot string
	ownerPID int64
	runID    string
	now      func() time.Time
	newID    func() string
	newToken func() string
}

type Lease struct {
	registry *Registry
	artifact storage.TemporaryArtifact
	mu       sync.Mutex
}

type markerData struct {
	ID    string                        `json:"id"`
	Kind  storage.TemporaryArtifactKind `json:"kind"`
	Token string                        `json:"token"`
}

func NewRegistry(config Config) *Registry {
	tempRoot := config.TempRoot
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	if absolute, err := filepath.Abs(tempRoot); err == nil {
		tempRoot = absolute
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	newID := config.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	newToken := config.NewToken
	if newToken == nil {
		newToken = uuid.NewString
	}
	runID := config.RunID
	if runID == "" {
		runID = uuid.NewString()
	}
	ownerPID := config.OwnerPID
	if ownerPID <= 0 {
		ownerPID = int64(os.Getpid())
	}
	return &Registry{
		store: config.Store, tempRoot: filepath.Clean(tempRoot), ownerPID: ownerPID,
		runID: runID, now: now, newID: newID, newToken: newToken,
	}
}

func (r *Registry) Create(
	ctx context.Context,
	kind storage.TemporaryArtifactKind,
	metadata json.RawMessage,
) (*Lease, error) {
	if r.store == nil {
		return nil, errors.New("temporary artifact store is required")
	}
	prefix, err := prefixForKind(kind)
	if err != nil {
		return nil, err
	}
	if err := requireRealRoot(r.tempRoot); err != nil {
		return nil, err
	}
	path, err := os.MkdirTemp(r.tempRoot, prefix+"*")
	if err != nil {
		return nil, fmt.Errorf("create temporary artifact directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("protect temporary artifact directory: %w", err)
	}
	lease, err := r.RegisterExisting(ctx, kind, path, metadata)
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	return lease, nil
}

func (r *Registry) RegisterExisting(
	ctx context.Context,
	kind storage.TemporaryArtifactKind,
	path string,
	metadata json.RawMessage,
) (*Lease, error) {
	if r.store == nil {
		return nil, errors.New("temporary artifact store is required")
	}
	prefix, err := prefixForKind(kind)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("resolve temporary artifact path: %w", err)
	}
	if filepath.Dir(path) != r.tempRoot {
		return nil, fmt.Errorf("temporary artifact path must be a direct child of %s", r.tempRoot)
	}
	if !strings.HasPrefix(filepath.Base(path), prefix) {
		return nil, fmt.Errorf("temporary artifact path must use prefix %q", prefix)
	}
	if err := r.validateDirectory(path); err != nil {
		return nil, err
	}
	id, token := r.newID(), r.newToken()
	if id == "" || token == "" {
		return nil, errors.New("temporary artifact identifiers must not be empty")
	}
	if err := writeMarker(filepath.Join(path, MarkerName), markerData{ID: id, Kind: kind, Token: token}); err != nil {
		return nil, fmt.Errorf("write temporary artifact marker: %w", err)
	}
	now := r.now().UTC()
	artifact := &storage.TemporaryArtifact{
		ID: id, Kind: kind, Path: path, MarkerToken: token,
		State: storage.TemporaryArtifactStateActive, OwnerPID: r.ownerPID,
		CreatedAt: now, LastHeartbeatAt: &now, Metadata: metadataWithRunID(metadata, r.runID),
	}
	if err := r.store.CreateTemporaryArtifact(ctx, artifact); err != nil {
		_ = os.Remove(filepath.Join(path, MarkerName))
		return nil, err
	}
	return &Lease{registry: r, artifact: *artifact}, nil
}

func (r *Registry) List(ctx context.Context) ([]storage.TemporaryArtifact, error) {
	if r.store == nil {
		return nil, errors.New("temporary artifact store is required")
	}
	return r.store.ListTemporaryArtifacts(ctx)
}

func (r *Registry) Get(ctx context.Context, id string) (storage.TemporaryArtifact, error) {
	if r.store == nil {
		return storage.TemporaryArtifact{}, errors.New("temporary artifact store is required")
	}
	return r.store.GetTemporaryArtifact(ctx, id)
}

func (r *Registry) Reconcile(ctx context.Context) error {
	artifacts, err := r.List(ctx)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if artifact.State != storage.TemporaryArtifactStateActive {
			continue
		}
		if artifact.OwnerPID == r.ownerPID && metadataRunID(artifact.Metadata) == r.runID {
			continue
		}
		if artifact.OwnerPID == r.ownerPID {
			if err := r.MarkAbandoned(ctx, artifact.ID, "owner process run is no longer active"); err != nil {
				return fmt.Errorf("reconcile temporary artifact %s: %w", artifact.ID, err)
			}
			continue
		}
		alive, known := processAlive(artifact.OwnerPID)
		if !known || alive {
			continue
		}
		if err := r.MarkAbandoned(ctx, artifact.ID, "owner process is no longer running"); err != nil {
			return fmt.Errorf("reconcile temporary artifact %s: %w", artifact.ID, err)
		}
	}
	return nil
}

func (r *Registry) MarkAbandoned(ctx context.Context, id, reason string) error {
	return r.transition(ctx, id, storage.TemporaryArtifactStateAbandoned, reason)
}

func (r *Registry) MarkClosed(ctx context.Context, id string) error {
	return r.transition(ctx, id, storage.TemporaryArtifactStateClosed, "")
}

func (r *Registry) MarkFailed(ctx context.Context, id, reason string) error {
	return r.transition(ctx, id, storage.TemporaryArtifactStateFailed, reason)
}

func (r *Registry) MarkQuarantined(ctx context.Context, id string) error {
	return r.transition(ctx, id, storage.TemporaryArtifactStateQuarantined, "")
}

func (r *Registry) MarkDeleted(ctx context.Context, id string) error {
	return r.transition(ctx, id, storage.TemporaryArtifactStateDeleted, "")
}

func (r *Registry) transition(
	ctx context.Context,
	id string,
	next storage.TemporaryArtifactState,
	lastError string,
) error {
	return r.transitionAt(ctx, id, next, lastError, r.now())
}

func (r *Registry) transitionAt(
	ctx context.Context,
	id string,
	next storage.TemporaryArtifactState,
	lastError string,
	at time.Time,
) error {
	if r.store == nil {
		return errors.New("temporary artifact store is required")
	}
	return r.store.TransitionTemporaryArtifact(ctx, id, next, lastError, at.UTC())
}

func (r *Registry) Validate(artifact storage.TemporaryArtifact) error {
	prefix, err := prefixForKind(artifact.Kind)
	if err != nil {
		return err
	}
	if filepath.Clean(artifact.Path) != artifact.Path || filepath.Dir(artifact.Path) != r.tempRoot {
		return errors.New("temporary artifact path is outside the effective temp root")
	}
	if !strings.HasPrefix(filepath.Base(artifact.Path), prefix) {
		return errors.New("temporary artifact path has an unexpected prefix")
	}
	if err := r.validateDirectory(artifact.Path); err != nil {
		return err
	}
	return r.ValidateMarker(artifact.Path, artifact)
}

func (r *Registry) ValidateMarker(root string, artifact storage.TemporaryArtifact) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect temporary artifact marker parent: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("temporary artifact marker parent must be a real directory")
	}
	marker, err := readMarker(filepath.Join(root, MarkerName))
	if err != nil {
		return err
	}
	if marker.ID != artifact.ID || marker.Kind != artifact.Kind || marker.Token != artifact.MarkerToken {
		return errors.New("temporary artifact marker does not match the registry")
	}
	return nil
}

func (r *Registry) validateDirectory(path string) error {
	if err := storage.ValidateNoSymlinkPath(r.tempRoot, path); err != nil {
		return fmt.Errorf("validate temporary artifact path: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect temporary artifact path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("temporary artifact root must be a real directory")
	}
	return nil
}

func (r *Registry) TempRoot() string { return r.tempRoot }

func (l *Lease) Path() string { return l.artifact.Path }

func (l *Lease) Artifact() storage.TemporaryArtifact {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.artifact
}

func (l *Lease) Heartbeat(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.artifact.State != storage.TemporaryArtifactStateActive {
		return errors.New("temporary artifact lease is not active")
	}
	at := l.registry.now().UTC()
	if err := l.registry.store.HeartbeatTemporaryArtifact(ctx, l.artifact.ID, at); err != nil {
		return err
	}
	l.artifact.LastHeartbeatAt = &at
	return nil
}

func (l *Lease) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.artifact.State != storage.TemporaryArtifactStateActive {
		return nil
	}
	at := l.registry.now().UTC()
	if err := l.registry.transitionAt(
		ctx, l.artifact.ID, storage.TemporaryArtifactStateClosed, "", at,
	); err != nil {
		return err
	}
	l.artifact.State = storage.TemporaryArtifactStateClosed
	l.artifact.ClosedAt = &at
	return nil
}

func (l *Lease) Remove(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.artifact.State == storage.TemporaryArtifactStateDeleted {
		return nil
	}
	if err := l.registry.Validate(l.artifact); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.RemoveAll(l.artifact.Path); err != nil {
		return fmt.Errorf("remove temporary artifact: %w", err)
	}
	if err := l.registry.MarkDeleted(ctx, l.artifact.ID); err != nil {
		return err
	}
	l.artifact.State = storage.TemporaryArtifactStateDeleted
	return nil
}

func metadataWithRunID(metadata json.RawMessage, runID string) json.RawMessage {
	fields := make(map[string]json.RawMessage)
	trimmed := strings.TrimSpace(string(metadata))
	if trimmed != "" && trimmed != "null" {
		if err := json.Unmarshal(metadata, &fields); err != nil || fields == nil {
			encoded, _ := json.Marshal(string(metadata))
			fields = map[string]json.RawMessage{"_kandev_metadata": encoded}
		}
	}
	encodedRunID, _ := json.Marshal(runID)
	fields[registryRunIDMetadataKey] = encodedRunID
	encoded, _ := json.Marshal(fields)
	return encoded
}

func metadataRunID(metadata json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &fields); err != nil || fields == nil {
		return ""
	}
	var runID string
	if err := json.Unmarshal(fields[registryRunIDMetadataKey], &runID); err != nil {
		return ""
	}
	return runID
}

func writeMarker(path string, marker markerData) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("temporary artifact marker is not a regular file")
		}
		return errors.New("temporary artifact marker already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp := path + ".tmp-" + marker.Token
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = os.Remove(tmp)
		}
	}()
	if err := json.NewEncoder(file).Encode(marker); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func readMarker(path string) (markerData, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return markerData{}, fmt.Errorf("inspect temporary artifact marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return markerData{}, errors.New("temporary artifact marker is not owner-only regular data")
	}
	file, err := os.Open(path)
	if err != nil {
		return markerData{}, err
	}
	defer func() { _ = file.Close() }()
	var marker markerData
	if err := json.NewDecoder(file).Decode(&marker); err != nil {
		return markerData{}, fmt.Errorf("decode temporary artifact marker: %w", err)
	}
	return marker, nil
}

func prefixForKind(kind storage.TemporaryArtifactKind) (string, error) {
	switch kind {
	case storage.TemporaryArtifactKindImproveBundle:
		return "kandev-improve-", nil
	case storage.TemporaryArtifactKindHostUtility:
		return "kandev-host-utility-", nil
	default:
		return "", fmt.Errorf("unknown temporary artifact kind %q", kind)
	}
}

func requireRealRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect temporary artifact root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("temporary artifact root is not a real directory: %s", root)
	}
	return nil
}
