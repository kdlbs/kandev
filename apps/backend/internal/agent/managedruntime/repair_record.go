package managedruntime

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var repairOpaqueIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type repairSelection struct {
	SchemaVersion int    `json:"schema_version"`
	GenerationID  string `json:"generation_id"`
	PackageSpec   string `json:"package_spec"`
	ContextDigest string `json:"context_digest"`
	CreatedAt     string `json:"created_at"`
	MAC           string `json:"mac"`
}

type repairGeneration struct {
	SchemaVersion int    `json:"schema_version"`
	GenerationID  string `json:"generation_id"`
	OwnerID       string `json:"owner_id"`
	PackageSpec   string `json:"package_spec"`
	ContextDigest string `json:"context_digest"`
	CreatedAt     string `json:"created_at"`
	MAC           string `json:"mac"`
}

func (s *RepairStore) createGeneration(repairContext RepairContext, digest string) (string, string, string, error) {
	generationID, err := newRepairRandomID(16)
	if err != nil {
		return "", "", "", err
	}
	ownerID, err := newRepairRandomID(16)
	if err != nil {
		return "", "", "", err
	}
	generationDir := filepath.Join(s.root, repairStoreGenerationDir, generationID)
	if err := os.Mkdir(generationDir, 0o700); err != nil {
		return "", "", "", fmt.Errorf("create managed runtime repair generation: %w", err)
	}
	record := repairGeneration{
		SchemaVersion: RepairContextSchemaVersion,
		GenerationID:  generationID,
		OwnerID:       ownerID,
		PackageSpec:   repairContext.PackageSpec,
		ContextDigest: digest,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := signRepairRecord(s.key, &record); err != nil {
		_ = os.RemoveAll(generationDir)
		return "", "", "", err
	}
	if err := writeRepairRecord(filepath.Join(generationDir, repairGenerationRecord), record); err != nil {
		_ = os.RemoveAll(generationDir)
		return "", "", "", err
	}
	return generationID, generationDir, ownerID, nil
}

func (s *RepairStore) readSelection(repairContext RepairContext, digest string) (*PreparedRuntime, RepairOutcome, error) {
	if s == nil || !validRepairHexDigest(digest) {
		return nil, RepairOutcomeMiss, nil
	}
	selection, outcome := s.readSelectionRecord(repairContext, digest)
	if outcome != RepairOutcomeHit {
		return nil, outcome, nil
	}
	generationDir := filepath.Join(s.root, repairStoreGenerationDir, selection.GenerationID)
	record, valid := readValidRepairGeneration(generationDir, selection, repairContext, digest, s.key)
	if !valid {
		return nil, RepairOutcomeInvalid, nil
	}
	prepared := newPreparedRepairRuntime(generationDir, repairContext.PackageSpec, record.OwnerID)
	if err := prepared.validate(); err != nil {
		return nil, RepairOutcomeInvalid, nil
	}
	prepared.repair = &preparedRuntimeRepair{
		store: s, digest: digest, generationID: selection.GenerationID, ownerID: record.OwnerID, published: true,
	}
	return prepared, RepairOutcomeHit, nil
}

func (s *RepairStore) readSelectionRecord(repairContext RepairContext, digest string) (repairSelection, RepairOutcome) {
	selectionPath := filepath.Join(s.root, repairStoreContextsDir, digest, repairSelectionRecord)
	if !repairRecordExists(selectionPath) {
		return repairSelection{}, RepairOutcomeMiss
	}
	selection, ok := readRepairSelection(selectionPath, s.key)
	if !ok || selection.ContextDigest != digest || selection.PackageSpec != repairContext.PackageSpec ||
		!repairOpaqueIDPattern.MatchString(selection.GenerationID) {
		return repairSelection{}, RepairOutcomeInvalid
	}
	return selection, RepairOutcomeHit
}

func readValidRepairGeneration(
	generationDir string,
	selection repairSelection,
	repairContext RepairContext,
	digest string,
	key []byte,
) (repairGeneration, bool) {
	recordPath := filepath.Join(generationDir, repairGenerationRecord)
	if !repairRecordExists(recordPath) {
		return repairGeneration{}, false
	}
	record, ok := readRepairGeneration(recordPath, key)
	if !ok || record.GenerationID != selection.GenerationID || record.ContextDigest != digest ||
		record.PackageSpec != repairContext.PackageSpec || !repairOpaqueIDPattern.MatchString(record.OwnerID) {
		return repairGeneration{}, false
	}
	return record, validRepairGenerationTree(generationDir, record.OwnerID)
}

func newPreparedRepairRuntime(root, packageSpec, ownerID string) *PreparedRuntime {
	return &PreparedRuntime{
		root: root, project: filepath.Join(root, "project"), cache: filepath.Join(root, "cache"),
		packageSpec: packageSpec, ownerID: ownerID,
	}
}

func readRepairSelection(path string, key []byte) (repairSelection, bool) {
	var selection repairSelection
	data, ok := readBoundedRepairRecord(path)
	if !ok || json.Unmarshal(data, &selection) != nil || !verifyRepairRecordMAC(key, selection) {
		return selection, false
	}
	if selection.SchemaVersion != RepairContextSchemaVersion || selection.CreatedAt == "" {
		return selection, false
	}
	return selection, true
}

func readRepairGeneration(path string, key []byte) (repairGeneration, bool) {
	var record repairGeneration
	data, ok := readBoundedRepairRecord(path)
	if !ok || json.Unmarshal(data, &record) != nil || !verifyRepairRecordMAC(key, record) {
		return record, false
	}
	if record.SchemaVersion != RepairContextSchemaVersion || record.CreatedAt == "" {
		return record, false
	}
	return record, true
}

func repairRecordExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func readBoundedRepairRecord(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxRepairNPMConfigBytes {
		return nil, false
	}
	if validatePrivateRepairPermissions(info) != nil || validatePrivateRepairOwnership(info) != nil {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxRepairNPMConfigBytes+1))
	if err != nil || len(data) > maxRepairNPMConfigBytes {
		return nil, false
	}
	return data, true
}

func signRepairRecord(key []byte, record any) error {
	switch value := record.(type) {
	case *repairSelection:
		value.MAC = ""
		mac, err := repairHMACHex(key, mustRepairRecordJSON(*value))
		if err != nil {
			return err
		}
		value.MAC = mac
	case *repairGeneration:
		value.MAC = ""
		mac, err := repairHMACHex(key, mustRepairRecordJSON(*value))
		if err != nil {
			return err
		}
		value.MAC = mac
	default:
		return errors.New("unsupported managed runtime repair record")
	}
	return nil
}

func verifyRepairRecordMAC(key []byte, record any) bool {
	switch value := record.(type) {
	case repairSelection:
		expected := value.MAC
		value.MAC = ""
		mac, err := repairHMACHex(key, mustRepairRecordJSON(value))
		return err == nil && expected != "" && hmac.Equal([]byte(expected), []byte(mac))
	case repairGeneration:
		expected := value.MAC
		value.MAC = ""
		mac, err := repairHMACHex(key, mustRepairRecordJSON(value))
		return err == nil && expected != "" && hmac.Equal([]byte(expected), []byte(mac))
	default:
		return false
	}
}

func mustRepairRecordJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func writeRepairRecord(path string, record any) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode managed runtime repair record: %w", err)
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".repair-record-*")
	if err != nil {
		return fmt.Errorf("create managed runtime repair record: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("secure managed runtime repair record: %w", err)
	}
	_, writeErr := temporary.Write(encoded)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		cleanup()
		return fmt.Errorf("write managed runtime repair record: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		cleanup()
		return fmt.Errorf("publish managed runtime repair record: %w", err)
	}
	syncRepairDirectory(directory)
	return nil
}

func (s *RepairStore) writeSelection(digest string, prepared *PreparedRuntime) error {
	contextDir := filepath.Join(s.root, repairStoreContextsDir, digest)
	if err := ensurePrivateRepairDirectory(contextDir); err != nil {
		return err
	}
	selection := repairSelection{
		SchemaVersion: RepairContextSchemaVersion,
		GenerationID:  prepared.repair.generationID,
		PackageSpec:   prepared.PackageSpec(),
		ContextDigest: digest,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := signRepairRecord(s.key, &selection); err != nil {
		return err
	}
	return writeRepairRecord(filepath.Join(contextDir, repairSelectionRecord), selection)
}

func validRepairGenerationTree(generationDir, ownerID string) bool {
	info, err := os.Lstat(generationDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	marker, err := os.ReadFile(filepath.Join(generationDir, preparedRuntimeOwnerMarker))
	if err != nil || string(marker) != ownerID {
		return false
	}
	for _, name := range []string{"project", "cache"} {
		child := filepath.Join(generationDir, name)
		childInfo, err := os.Lstat(child)
		if err != nil || !childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

// removeRepairGeneration removes only one exact unpublished owner root.
func removeRepairGeneration(generationDir, ownerID string) error {
	info, err := os.Lstat(generationDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect managed runtime repair generation: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed runtime repair generation is not a real directory")
	}
	marker, err := os.ReadFile(filepath.Join(generationDir, preparedRuntimeOwnerMarker))
	if err != nil || string(marker) != ownerID {
		return errors.New("managed runtime repair generation owner does not match")
	}
	if err := os.RemoveAll(generationDir); err != nil {
		return fmt.Errorf("remove managed runtime repair generation: %w", err)
	}
	return nil
}

func (l *RepairLease) release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if l.released {
		l.mu.Unlock()
		return nil
	}
	l.released = true
	lock := l.lock
	l.lock = nil
	l.mu.Unlock()
	if lock == nil {
		return nil
	}
	unlockErr := unlockRepairContextLock(lock)
	closeErr := lock.Close()
	return errors.Join(unlockErr, closeErr)
}

func (l *RepairLease) isRetired() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.released || l.prepared == nil
}

func (l *RepairLease) isOwned() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.released && l.prepared != nil
}

// Close releases the context lock. It never removes a prepared generation;
// the PreparedRuntime owner decides that lifetime.
func (l *RepairLease) Close() error {
	if l == nil {
		return nil
	}
	return l.release()
}

// preparedRuntimeRepair binds one generation handle to its store owner.
type preparedRuntimeRepair struct {
	store        *RepairStore
	lease        *RepairLease
	digest       string
	generationID string
	ownerID      string
	mu           sync.Mutex
	published    bool
	closed       bool
}

func (r *preparedRuntimeRepair) isPublished() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.published
}

func (r *preparedRuntimeRepair) retire() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.published || r.closed {
		return
	}
	r.closed = true
	if r.lease != nil {
		_ = r.lease.release()
	}
}

func (r *preparedRuntimeRepair) publish(prepared *PreparedRuntime) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.published {
		return nil
	}
	if r.closed {
		return errors.New("managed runtime repair owner is closed")
	}
	if r.store == nil || r.lease == nil || r.digest == "" {
		return errors.New("managed runtime repair publication owner is unavailable")
	}
	if r.lease.isRetired() || !r.lease.isOwned() {
		return errors.New("managed runtime repair publication owner is retired")
	}
	if err := prepared.validate(); err != nil {
		r.closed = true
		_ = r.lease.release()
		return fmt.Errorf("validated repair files are unavailable: %w", err)
	}
	if err := r.store.writeSelection(r.digest, prepared); err != nil {
		r.closed = true
		_ = r.lease.release()
		return err
	}
	r.published = true
	return r.lease.release()
}

func (r *preparedRuntimeRepair) closeUnpublished(prepared *PreparedRuntime) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.published {
		return nil
	}
	r.closed = true
	if prepared == nil || r.store == nil {
		return nil
	}
	if err := removeRepairGeneration(prepared.root, r.ownerID); err != nil {
		r.closed = false
		return err
	}
	if r.lease != nil {
		return r.lease.release()
	}
	return nil
}

func (r *preparedRuntimeRepair) releaseLease() {
	if r == nil || r.lease == nil {
		return
	}
	_ = r.lease.release()
}
