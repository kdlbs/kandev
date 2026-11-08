package managedruntime

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	repairStoreRootName      = "managed-runtime-repairs"
	repairStoreVersion       = "v1"
	repairStoreOwnerMarker   = "owner"
	repairStoreKeyFile       = "fingerprint.key"
	repairStoreContextsDir   = "contexts"
	repairStoreGenerationDir = "generations"
	repairGenerationRecord   = "generation.json"
	repairSelectionRecord    = "selection.json"
	repairContextLockFile    = "lock"
	repairKeyBytes           = 32
)

// RepairOutcome is the closed result of one reusable-repair selection attempt.
type RepairOutcome string

const (
	RepairOutcomeMiss    RepairOutcome = "miss"
	RepairOutcomeHit     RepairOutcome = "hit"
	RepairOutcomeInvalid RepairOutcome = "invalid"
)

// RepairStore owns validated reusable generations on one execution host.
// Published generations survive every owner handle; unpublished candidates
// are removed by their owner after their consumers stop.
type RepairStore struct {
	root  string
	owner string
	key   []byte
}

// OpenRepairStore opens the executor-local reusable repair store beneath the
// resolved Kandev home. An unavailable or unsafe store returns an error and
// the caller keeps the existing private recovery path.
func OpenRepairStore(homeDir string, excludedRoots ...string) (*RepairStore, error) {
	root, err := repairStoreRoot(homeDir)
	if err != nil {
		return nil, err
	}
	if err := rejectRepairStoreExclusions(root, excludedRoots); err != nil {
		return nil, err
	}
	if err := ensureRepairStoreHome(homeDir); err != nil {
		return nil, err
	}
	ownerRoot := filepath.Dir(root)
	for _, path := range []string{ownerRoot, root, filepath.Join(root, repairStoreContextsDir), filepath.Join(root, repairStoreGenerationDir)} {
		if err := ensurePrivateRepairDirectory(path); err != nil {
			return nil, err
		}
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve managed runtime repair store: %w", err)
	}
	if resolved != root {
		return nil, errors.New("managed runtime repair store resolves through a symlink")
	}
	owner, err := ensureRepairOwnerMarker(root)
	if err != nil {
		return nil, err
	}
	key, err := loadOrCreateRepairKey(filepath.Join(root, repairStoreKeyFile))
	if err != nil {
		return nil, err
	}
	return &RepairStore{root: root, owner: owner, key: key}, nil
}

// RepairRoot reports the contained store root for diagnostics only. It never
// enters generation metadata or logs that leave the executor.
func (s *RepairStore) RepairRoot() string {
	if s == nil {
		return ""
	}
	return s.root
}

func rejectRepairStoreExclusions(root string, excludedRoots []string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		resolvedRoot = filepath.Clean(root)
	} else if err != nil {
		return fmt.Errorf("resolve managed runtime repair store: %w", err)
	}
	for _, excluded := range excludedRoots {
		excluded = strings.TrimSpace(excluded)
		if excluded == "" || !filepath.IsAbs(excluded) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(excluded)
		if errors.Is(err, os.ErrNotExist) {
			resolved = filepath.Clean(excluded)
		} else if err != nil {
			continue
		}
		if pathWithin(resolved, resolvedRoot) || pathWithin(resolvedRoot, resolved) {
			return errors.New("managed runtime repair store is inside an excluded workspace root")
		}
	}
	return nil
}

func readBoundedRepairIdentityFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > limit {
		return nil, errors.New("managed runtime repair identity file is not a bounded regular file")
	}
	if err := validatePrivateRepairPermissions(info); err != nil {
		return nil, err
	}
	if err := validatePrivateRepairOwnership(info); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(contents)) > limit {
		return nil, errors.New("managed runtime repair identity file is unreadable")
	}
	return contents, nil
}

func repairStoreRoot(homeDir string) (string, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" || !filepath.IsAbs(homeDir) {
		return "", errors.New("managed runtime repair store home is not absolute")
	}
	clean := filepath.Clean(homeDir)
	if clean == string(filepath.Separator) {
		return "", errors.New("managed runtime repair store home is the filesystem root")
	}
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		return filepath.Join(clean, repairStoreRootName, repairStoreVersion), nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect managed runtime repair store home: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("managed runtime repair store home is not a real directory")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("resolve managed runtime repair store home: %w", err)
	}
	return filepath.Join(resolved, repairStoreRootName, repairStoreVersion), nil
}

func ensureRepairStoreHome(homeDir string) error {
	clean := filepath.Clean(strings.TrimSpace(homeDir))
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		if mkdirErr := os.MkdirAll(clean, 0o700); mkdirErr != nil {
			return fmt.Errorf("create managed runtime repair store home: %w", mkdirErr)
		}
		info, err = os.Lstat(clean)
	}
	if err != nil {
		return fmt.Errorf("inspect managed runtime repair store home: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed runtime repair store home is not a real directory")
	}
	return validatePrivateRepairOwnership(info)
}

func ensurePrivateRepairDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		mkdirErr := os.Mkdir(path, 0o700)
		if mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
			return fmt.Errorf("create managed runtime repair directory: %w", mkdirErr)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect managed runtime repair directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed runtime repair path is not a real directory")
	}
	if err := validatePrivateRepairPermissions(info); err != nil {
		return err
	}
	return validatePrivateRepairOwnership(info)
}

func ensureRepairOwnerMarker(root string) (string, error) {
	path := filepath.Join(root, repairStoreOwnerMarker)
	contents, err := readBoundedRepairIdentityFile(path, 128)
	if err == nil {
		owner := strings.TrimSpace(string(contents))
		if !repairOpaqueIDPattern.MatchString(owner) {
			return "", errors.New("managed runtime repair store owner marker is invalid")
		}
		return owner, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read managed runtime repair store owner marker: %w", err)
	}
	owner, err := newRepairRandomID(16)
	if err != nil {
		return "", err
	}
	if err := createExclusivePrivateFile(path, []byte(owner)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ensureRepairOwnerMarker(root)
		}
		return "", fmt.Errorf("create managed runtime repair store owner marker: %w", err)
	}
	return owner, nil
}

func loadOrCreateRepairKey(path string) ([]byte, error) {
	contents, err := readBoundedRepairIdentityFile(path, 128)
	if err == nil {
		return decodeRepairKey(contents)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read managed runtime repair fingerprint key: %w", err)
	}
	key := make([]byte, repairKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("create managed runtime repair fingerprint key: %w", err)
	}
	if err := createExclusivePrivateFile(path, []byte(hex.EncodeToString(key))); err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadOrCreateRepairKey(path)
		}
		return nil, fmt.Errorf("create managed runtime repair fingerprint key: %w", err)
	}
	return key, nil
}

func decodeRepairKey(contents []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(contents))
	key, err := hex.DecodeString(trimmed)
	if err != nil || len(key) != repairKeyBytes {
		return nil, errors.New("managed runtime repair fingerprint key is malformed")
	}
	return key, nil
}

func createExclusivePrivateFile(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(contents)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func newRepairRandomID(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func repairHMACHex(key, value []byte) (string, error) {
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Context completes one trusted installation input with the executor-local
// environment used to identify a compatible generation.
func (s *RepairStore) Context(input RepairContextInput) (RepairContext, error) {
	if s == nil {
		return RepairContext{}, errors.New("managed runtime repair store is unavailable")
	}
	local, err := resolveRepairLocalEnvironment(input.Env, input.InheritedEnv)
	if err != nil {
		return RepairContext{}, err
	}
	return newRepairContext(input, local)
}

// Lookup returns a validated published generation for the context. A missing
// or invalid record never authorizes execution and never removes files.
func (s *RepairStore) Lookup(ctx context.Context, repairContext RepairContext) (*PreparedRuntime, RepairOutcome, error) {
	if s == nil {
		return nil, RepairOutcomeMiss, errors.New("managed runtime repair store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, RepairOutcomeMiss, err
	}
	digest, err := repairContext.digest(s.key)
	if err != nil {
		return nil, RepairOutcomeMiss, err
	}
	prepared, outcome, err := s.readSelection(repairContext, digest)
	if err != nil || outcome != RepairOutcomeHit {
		return nil, outcome, err
	}
	return prepared, outcome, nil
}

// BeginRecovery acquires the per-context OS lock and returns an owner lease.
// The caller reuses an existing generation or prepares a new candidate and
// publishes it only after successful protocol initialization.
func (s *RepairStore) BeginRecovery(ctx context.Context, repairContext RepairContext) (*RepairLease, error) {
	if s == nil {
		return nil, errors.New("managed runtime repair store is unavailable")
	}
	digest, err := repairContext.digest(s.key)
	if err != nil {
		return nil, err
	}
	contextDir := filepath.Join(s.root, repairStoreContextsDir, digest)
	if err := ensurePrivateRepairDirectory(contextDir); err != nil {
		return nil, err
	}
	lock, err := openRepairContextLock(filepath.Join(contextDir, repairContextLockFile))
	if err != nil {
		return nil, err
	}
	if err := acquireRepairContextLock(ctx, lock); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return &RepairLease{store: s, context: repairContext, digest: digest, lock: lock}, nil
}

// RepairLease owns one per-context lock while a caller rechecks selection and
// prepares a candidate. The lock is released by Publish or Close.
type RepairLease struct {
	store    *RepairStore
	context  RepairContext
	digest   string
	lock     *os.File
	prepared *PreparedRuntime
	mu       sync.Mutex
	released bool
}

// Reuse rechecks selection under the held lock. A valid generation releases
// the lock immediately and is returned for its consumer.
func (l *RepairLease) Reuse(ctx context.Context) (*PreparedRuntime, RepairOutcome, error) {
	if l == nil {
		return nil, RepairOutcomeMiss, errors.New("managed runtime repair lease is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, RepairOutcomeMiss, err
	}
	prepared, outcome, err := l.store.readSelection(l.context, l.digest)
	if err != nil || outcome != RepairOutcomeHit {
		return nil, outcome, err
	}
	if releaseErr := l.release(); releaseErr != nil {
		_ = prepared.Close()
		return nil, RepairOutcomeMiss, releaseErr
	}
	return prepared, RepairOutcomeHit, nil
}

// Prepare installs one new unpublished generation at its final stable
// directory. The per-context lock stays held until Publish or Close.
func (l *RepairLease) Prepare(
	ctx context.Context,
	env []string,
	run ManagedRuntimeCommandRunner,
) (*PreparedRuntime, error) {
	if l == nil {
		return nil, errors.New("managed runtime repair lease is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	generationID, generationDir, ownerID, err := l.store.createGeneration(l.context, l.digest)
	if err != nil {
		return nil, err
	}
	repair := &preparedRuntimeRepair{
		store: l.store, lease: l, digest: l.digest, generationID: generationID, ownerID: ownerID,
	}
	prepared, err := allocateOwnedRuntime(ctx, generationDir, ownerID, l.context.PackageSpec, env, run, repair)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.prepared = prepared
	l.mu.Unlock()
	return prepared, nil
}

// Publish atomically selects the prepared generation after successful
// protocol initialization.
func (p *PreparedRuntime) PublishRepair() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	repair := p.repair
	p.mu.Unlock()
	if repair == nil {
		return nil
	}
	return repair.publish(p)
}

// RepairPublished reports whether this handle already names a selected
// generation. Published handles never remove their files.
func (p *PreparedRuntime) RepairPublished() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	repair := p.repair
	p.mu.Unlock()
	return repair.isPublished()
}

// MatchesRepairContext reports whether a handle belongs to the supplied
// installation context. It never reads or mutates generation files.
func (s *RepairStore) MatchesRepairContext(prepared *PreparedRuntime, repairContext RepairContext) bool {
	if s == nil || prepared == nil {
		return false
	}
	digest, err := repairContext.digest(s.key)
	if err != nil {
		return false
	}
	prepared.mu.Lock()
	repair := prepared.repair
	prepared.mu.Unlock()
	if repair == nil {
		return false
	}
	repair.mu.Lock()
	defer repair.mu.Unlock()
	return repair.digest == digest
}

// RetireRepairPublication abandons an unpublished candidate at its caller's
// deadline. The current process keeps every file it uses; teardown removes
// the candidate once all consumers stop.
func (p *PreparedRuntime) RetireRepairPublication() {
	if p == nil {
		return
	}
	p.mu.Lock()
	repair := p.repair
	p.mu.Unlock()
	if repair == nil {
		return
	}
	repair.retire()
}
