package canvas

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	PreparationTTL         = 15 * time.Minute
	MaxPreparationsPerUser = 2
	MaxStagedBytes         = int64(256 << 20)
)

var (
	ErrPreparationNotFound = errors.New("canvas preparation not found")
	ErrPreparationExpired  = errors.New("canvas preparation expired")
	ErrPreparationLimit    = errors.New("canvas preparation limit reached")
	ErrPreparationInvalid  = errors.New("invalid canvas preparation")
)

type ExportKind string

const (
	ExportBundle ExportKind = "bundle"
	ExportSource ExportKind = "source"
)

// Preparation is temporary, user-bound distribution state. Bundle and Source
// are accepted only by Create and are not kept in the store's metadata map.
type Preparation struct {
	ID                   string
	UserID               string
	WorkspaceID          string
	CanvasID             string
	ReleaseID            string
	PackageID            string
	PackageVersion       string
	PackageDigest        string
	PackageArchiveDigest string
	Metadata             ExportMetadata
	SourceID             string
	OriginKind           string
	RepositoryURL        string
	Bundle               []byte
	Source               []byte
	Files                []ExportFile
	CreatedAt            time.Time
	ExpiresAt            time.Time
}

type ExportFile struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

type preparationRecord struct {
	Preparation
	BundleBytes int64
	SourceBytes int64
	leases      int
}

// PreparationStore stages files below a dedicated recovery boundary and keeps
// only bounded metadata in memory. It is intentionally process-local: startup
// cleanup invalidates every uncommitted preparation.
type PreparationStore struct {
	mu    sync.Mutex
	root  string
	clock func() time.Time
	items map[string]*preparationRecord
}

func NewPreparationStore(root string) (*PreparationStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrPreparationInvalid
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("canvas preparations: create root: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("canvas preparations: inspect root: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return nil, fmt.Errorf("canvas preparations: clear stale staging: %w", err)
		}
	}
	return &PreparationStore{root: root, clock: time.Now, items: make(map[string]*preparationRecord)}, nil
}

func (s *PreparationStore) SetClock(clock func() time.Time) {
	if s == nil || clock == nil {
		return
	}
	s.mu.Lock()
	s.clock = clock
	s.mu.Unlock()
}

func (s *PreparationStore) Create(ctx context.Context, input Preparation) (Preparation, error) {
	if err := validatePreparationCreate(ctx, s, input); err != nil {
		return Preparation{}, err
	}
	input, err := preparePreparationIdentity(input)
	if err != nil {
		return Preparation{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowLocked()
	if err := s.removeExpiredLocked(now); err != nil {
		return Preparation{}, fmt.Errorf("canvas preparations: remove expired: %w", err)
	}
	if err := s.validateCreateAdmissionLocked(input); err != nil {
		return Preparation{}, err
	}
	input, err = setPreparationTimes(input, now)
	if err != nil {
		return Preparation{}, err
	}
	if err := s.stagePreparation(input); err != nil {
		return Preparation{}, err
	}
	record := &preparationRecord{Preparation: input, BundleBytes: int64(len(input.Bundle)), SourceBytes: int64(len(input.Source))}
	record.Bundle = nil
	record.Source = nil
	s.items[input.ID] = record
	return input, nil
}

func validatePreparationCreate(ctx context.Context, store *PreparationStore, input Preparation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.WorkspaceID) == "" {
		return ErrPreparationInvalid
	}
	if len(input.Bundle) == 0 && len(input.Source) == 0 {
		return ErrPreparationInvalid
	}
	return nil
}

func preparePreparationIdentity(input Preparation) (Preparation, error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = uuid.NewString()
	}
	if !safePreparationID(input.ID) {
		return Preparation{}, ErrPreparationInvalid
	}
	return input, nil
}

func (s *PreparationStore) validateCreateAdmissionLocked(input Preparation) error {
	if _, exists := s.items[input.ID]; exists {
		return ErrPreparationInvalid
	}
	if s.userCountLocked(input.UserID) >= MaxPreparationsPerUser || s.stagedBytesLocked()+int64(len(input.Bundle))+int64(len(input.Source)) > MaxStagedBytes {
		return ErrPreparationLimit
	}
	return nil
}

func setPreparationTimes(input Preparation, now time.Time) (Preparation, error) {
	if input.CreatedAt.IsZero() {
		input.CreatedAt = now
	}
	if input.ExpiresAt.IsZero() {
		input.ExpiresAt = now.Add(PreparationTTL)
	}
	if !input.ExpiresAt.After(now) {
		return Preparation{}, ErrPreparationExpired
	}
	return input, nil
}

func (s *PreparationStore) stagePreparation(input Preparation) error {
	directory := filepath.Join(s.root, input.ID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("canvas preparations: create staging directory: %w", err)
	}
	if err := writePreparationFile(directory, "bundle", input.Bundle); err != nil {
		_ = os.RemoveAll(directory)
		return err
	}
	if err := writePreparationFile(directory, "source", input.Source); err != nil {
		_ = os.RemoveAll(directory)
		return err
	}
	return nil
}

func writePreparationFile(directory, name string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("canvas preparations: stage %s: %w", name, err)
	}
	return nil
}

func (s *PreparationStore) Get(ctx context.Context, userID, id string) (Preparation, error) {
	if err := ctx.Err(); err != nil {
		return Preparation{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.lookupLocked(userID, id)
	if err != nil {
		return Preparation{}, err
	}
	return record.Preparation, nil
}

func (s *PreparationStore) Open(ctx context.Context, userID, id string, kind ExportKind) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	record, err := s.lookupLocked(userID, id)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	record.leases++
	root := s.root
	s.mu.Unlock()
	defer s.releaseLease(id)
	name := string(kind)
	if kind != ExportBundle && kind != ExportSource {
		return nil, ErrPreparationInvalid
	}
	data, err := os.ReadFile(filepath.Join(root, id, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrPreparationNotFound
		}
		return nil, err
	}
	return data, nil
}

func (s *PreparationStore) Delete(ctx context.Context, userID, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.items[id]
	if !exists || record.UserID != userID {
		return nil
	}
	if record.leases > 0 {
		return nil
	}
	delete(s.items, id)
	return os.RemoveAll(filepath.Join(s.root, id))
}

func (s *PreparationStore) Cleanup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeExpiredLocked(s.nowLocked())
}

func (s *PreparationStore) releaseLease(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, exists := s.items[id]; exists && record.leases > 0 {
		record.leases--
	}
}

func (s *PreparationStore) lookupLocked(userID, id string) (*preparationRecord, error) {
	if !safePreparationID(id) {
		return nil, ErrPreparationNotFound
	}
	record, exists := s.items[id]
	if !exists || record.UserID != userID {
		return nil, ErrPreparationNotFound
	}
	if !record.ExpiresAt.After(s.nowLocked()) {
		if record.leases == 0 {
			delete(s.items, id)
			_ = os.RemoveAll(filepath.Join(s.root, id))
		}
		return nil, ErrPreparationExpired
	}
	return record, nil
}

func (s *PreparationStore) removeExpiredLocked(now time.Time) error {
	var firstErr error
	for id, record := range s.items {
		if record.leases > 0 || record.ExpiresAt.After(now) {
			continue
		}
		delete(s.items, id)
		if err := os.RemoveAll(filepath.Join(s.root, id)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *PreparationStore) userCountLocked(userID string) int {
	count := 0
	for _, record := range s.items {
		if record.UserID == userID {
			count++
		}
	}
	return count
}

func (s *PreparationStore) stagedBytesLocked() int64 {
	var total int64
	for _, record := range s.items {
		total += record.BundleBytes + record.SourceBytes
	}
	return total
}

func (s *PreparationStore) nowLocked() time.Time {
	if s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock().UTC()
}

func safePreparationID(id string) bool {
	return id != "" && filepath.Base(id) == id && !strings.ContainsAny(id, "/\\\x00")
}
