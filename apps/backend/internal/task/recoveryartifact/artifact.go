// Package recoveryartifact stores exact recovery-file ownership proofs.
package recoveryartifact

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

var ErrIdentityMismatch = errors.New("task environment recovery artifact identity mismatch")

const (
	ProvenanceV2Operation       = "v2_operation"
	ProvenanceLegacyAdmission   = "legacy_admission"
	ProvenanceLegacyPublished   = "legacy_published"
	maximumArtifactPathCount    = 32
	maximumArtifactPathByteSize = 16 * 1024
)

// Registration identifies the current inventory slot and the exact recovery
// paths authorized for that operation. ArtifactPaths may grow as a retry
// snapshot is prepared, but existing paths cannot be substituted.
type Registration struct {
	TaskEnvironmentID   string
	OwnerTaskID         string
	OwnershipGeneration int64
	SessionID           string
	OperationID         string
	ExecutorType        string
	WorktreeID          string
	RepositoryID        string
	OriginalPath        string
	ReplacementID       string
	ReplacementPath     string
	LayoutVersion       int
	Provenance          string
	ArtifactPaths       []string
	ArtifactIdentities  map[string]string
}

// Registered is the persisted artifact proof returned to trusted callers.
type Registered struct {
	Registration
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Register creates or extends an artifact proof while the exact recovery
// claim, owner generation, and canonical inventory slot are still current.
func Register(ctx context.Context, db *sqlx.DB, req Registration) error {
	if err := validateRegistration(req); err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := validateAuthorityTx(ctx, db, tx, req); err != nil {
		return err
	}
	paths, identities, err := prepareArtifactProofs(ctx, tx, db, req)
	if err != nil {
		return err
	}
	return persistArtifactRegistration(ctx, db, tx, req, paths, identities)
}

func prepareArtifactProofs(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	req Registration,
) ([]string, map[string]string, error) {
	paths, identities, err := loadArtifactProofs(ctx, tx, db, req)
	if err != nil {
		return nil, nil, err
	}
	merged, err := mergePaths(paths, req.ArtifactPaths)
	if err != nil {
		return nil, nil, err
	}
	if err := addRequestedArtifactIdentities(merged, identities, req); err != nil {
		return nil, nil, err
	}
	return merged, identities, nil
}

func addRequestedArtifactIdentities(
	mergedPaths []string,
	identities map[string]string,
	req Registration,
) error {
	// Existing paths keep their original identity unless trusted validation
	// presents a fresh identity for that exact current object.
	for _, path := range req.ArtifactPaths {
		if requestedIdentity := req.ArtifactIdentities[path]; requestedIdentity != "" {
			currentIdentity, ok := FilesystemIdentity(path)
			if !ok || currentIdentity != requestedIdentity {
				return ErrIdentityMismatch
			}
			identities[path] = requestedIdentity
			continue
		}
		if identities[path] != "" {
			continue
		}
		if identity, ok := FilesystemIdentity(path); ok {
			identities[path] = identity
		}
	}
	for path := range identities {
		if !containsPath(mergedPaths, path) {
			delete(identities, path)
		}
	}
	return validateArtifactIdentities(mergedPaths, identities)
}

func persistArtifactRegistration(
	ctx context.Context,
	db *sqlx.DB,
	tx *sqlx.Tx,
	req Registration,
	paths []string,
	identities map[string]string,
) error {
	encoded, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	encodedIdentities, err := json.Marshal(identities)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_environment_recovery_artifacts (
			task_environment_id, operation_id, worktree_id, owner_task_id,
			ownership_generation, session_id, executor_type, repository_id,
			original_path, replacement_id, replacement_path, layout_version,
			provenance, artifact_paths_json, artifact_identities_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_environment_id, operation_id, worktree_id) DO UPDATE SET
			artifact_paths_json = excluded.artifact_paths_json,
			artifact_identities_json = excluded.artifact_identities_json,
			updated_at = excluded.updated_at
		WHERE task_environment_recovery_artifacts.owner_task_id = excluded.owner_task_id
		  AND task_environment_recovery_artifacts.ownership_generation = excluded.ownership_generation
		  AND task_environment_recovery_artifacts.session_id = excluded.session_id
		  AND task_environment_recovery_artifacts.executor_type = excluded.executor_type
		  AND task_environment_recovery_artifacts.repository_id = excluded.repository_id
		  AND task_environment_recovery_artifacts.original_path = excluded.original_path
		  AND task_environment_recovery_artifacts.replacement_id = excluded.replacement_id
		  AND task_environment_recovery_artifacts.replacement_path = excluded.replacement_path
		  AND task_environment_recovery_artifacts.layout_version = excluded.layout_version
		  AND task_environment_recovery_artifacts.provenance = excluded.provenance
	`), req.TaskEnvironmentID, req.OperationID, req.WorktreeID, req.OwnerTaskID,
		req.OwnershipGeneration, req.SessionID, req.ExecutorType, req.RepositoryID,
		req.OriginalPath, req.ReplacementID, req.ReplacementPath, req.LayoutVersion,
		req.Provenance, string(encoded), string(encodedIdentities), now, now)
	if err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT COUNT(1) FROM task_environment_recovery_artifacts
		WHERE task_environment_id = ? AND operation_id = ? AND worktree_id = ?
		  AND owner_task_id = ? AND ownership_generation = ? AND session_id = ?
		  AND executor_type = ? AND repository_id = ? AND original_path = ?
		  AND replacement_id = ? AND replacement_path = ? AND layout_version = ?
		  AND provenance = ? AND artifact_paths_json = ? AND artifact_identities_json = ?
	`), req.TaskEnvironmentID, req.OperationID, req.WorktreeID, req.OwnerTaskID,
		req.OwnershipGeneration, req.SessionID, req.ExecutorType, req.RepositoryID,
		req.OriginalPath, req.ReplacementID, req.ReplacementPath, req.LayoutVersion,
		req.Provenance, string(encoded), string(encodedIdentities)).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrIdentityMismatch
	}
	return tx.Commit()
}

// ListForEnvironment returns only rows whose ownership generation and
// published inventory identity remain current.
func ListForEnvironment(ctx context.Context, db *sqlx.DB, environmentID string) ([]Registered, error) {
	rows, err := db.QueryxContext(ctx, db.Rebind(`
		SELECT a.task_environment_id, a.owner_task_id, a.ownership_generation,
			a.session_id, a.operation_id, a.executor_type, a.worktree_id,
			a.repository_id, a.original_path, a.replacement_id, a.replacement_path,
			a.layout_version, a.provenance, a.artifact_paths_json, a.artifact_identities_json,
			a.created_at, a.updated_at
		FROM task_environment_recovery_artifacts a
		JOIN task_environments e ON e.id = a.task_environment_id
		JOIN task_environment_repos ter ON ter.task_environment_id = a.task_environment_id
		  AND ter.repository_id = a.repository_id
		  AND ((ter.worktree_id = a.worktree_id AND ter.worktree_path = a.original_path)
		    OR (ter.worktree_id = a.replacement_id AND ter.worktree_path = a.replacement_path))
		WHERE a.task_environment_id = ? AND e.task_id = a.owner_task_id
		  AND e.ownership_generation = a.ownership_generation
		  AND ter.deleted_at IS NULL AND ter.status NOT IN ('failed', 'deleted')
		ORDER BY a.created_at, a.worktree_id
	`), environmentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var registered []Registered
	for rows.Next() {
		var item Registered
		var pathsJSON, identitiesJSON string
		if err := rows.Scan(&item.TaskEnvironmentID, &item.OwnerTaskID, &item.OwnershipGeneration,
			&item.SessionID, &item.OperationID, &item.ExecutorType, &item.WorktreeID,
			&item.RepositoryID, &item.OriginalPath, &item.ReplacementID, &item.ReplacementPath,
			&item.LayoutVersion, &item.Provenance, &pathsJSON, &identitiesJSON,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(pathsJSON), &item.ArtifactPaths); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(identitiesJSON), &item.ArtifactIdentities); err != nil {
			continue
		}
		if err := validateRegistration(item.Registration); err != nil {
			continue
		}
		registered = append(registered, item)
	}
	return registered, rows.Err()
}

func validateRegistration(req Registration) error {
	if !hasRequiredRegistrationIdentity(req) {
		return ErrIdentityMismatch
	}
	if !validRegistrationProvenance(req.Provenance, req.LayoutVersion) {
		return ErrIdentityMismatch
	}
	if err := validateRegistrationPaths(req.ArtifactPaths); err != nil {
		return err
	}
	return validateArtifactIdentities(req.ArtifactPaths, req.ArtifactIdentities)
}

func hasRequiredRegistrationIdentity(req Registration) bool {
	return req.TaskEnvironmentID != "" && req.OwnerTaskID != "" && req.OwnershipGeneration >= 1 &&
		req.SessionID != "" && req.OperationID != "" && req.ExecutorType != "" &&
		req.WorktreeID != "" && req.RepositoryID != "" && req.OriginalPath != "" &&
		req.ReplacementID != "" && req.ReplacementPath != "" && req.LayoutVersion >= 1 && req.LayoutVersion <= 2
}

func validRegistrationProvenance(provenance string, layoutVersion int) bool {
	switch provenance {
	case ProvenanceV2Operation:
		return layoutVersion == 2
	case ProvenanceLegacyAdmission, ProvenanceLegacyPublished:
		return layoutVersion == 1
	default:
		return false
	}
}

func validateRegistrationPaths(paths []string) error {
	if len(paths) == 0 || len(paths) > maximumArtifactPathCount {
		return ErrIdentityMismatch
	}
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
			return ErrIdentityMismatch
		}
	}
	return nil
}

// FilesystemIdentity returns an opaque stable identity for a regular file or
// directory without following symlinks. Platforms without device/inode data
// fail open and return false, so workspace browsing can keep the path visible.
func FilesystemIdentity(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return "", false
	}
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return "", false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}
	device, deviceOK := reflectedUnsignedField(value, "Dev")
	inode, inodeOK := reflectedUnsignedField(value, "Ino")
	if !deviceOK || !inodeOK {
		return "", false
	}
	identity := fmt.Sprintf("%d:%d:%d", device, inode, uint32(info.Mode().Type()))
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:]), true
}

func reflectedUnsignedField(value reflect.Value, name string) (uint64, bool) {
	field := value.FieldByName(name)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() < 0 {
			return 0, false
		}
		return uint64(field.Int()), true
	default:
		return 0, false
	}
}

func validateArtifactIdentities(paths []string, identities map[string]string) error {
	if len(identities) > len(paths) {
		return ErrIdentityMismatch
	}
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		pathSet[path] = struct{}{}
	}
	for path, identity := range identities {
		if _, ok := pathSet[path]; !ok || len(identity) != sha256.Size*2 {
			return ErrIdentityMismatch
		}
		if _, err := hex.DecodeString(identity); err != nil {
			return ErrIdentityMismatch
		}
	}
	return nil
}

// VerifiedLegacyArtifactPaths returns only legacy paths whose current object
// still matches the identity captured under a recovery claim.
func VerifiedLegacyArtifactPaths(registered []Registered) []string {
	paths := make(map[string]struct{})
	for _, item := range registered {
		if item.Provenance != ProvenanceLegacyAdmission && item.Provenance != ProvenanceLegacyPublished {
			continue
		}
		for _, path := range item.ArtifactPaths {
			if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
				continue
			}
			registeredIdentity := item.ArtifactIdentities[path]
			if registeredIdentity == "" {
				continue
			}
			currentIdentity, ok := FilesystemIdentity(path)
			if ok && currentIdentity == registeredIdentity {
				paths[path] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func validateAuthorityTx(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, req Registration) error {
	ownerTaskID, err := loadRecoveryEnvironmentOwner(ctx, db, tx, req.TaskEnvironmentID)
	if err != nil {
		return err
	}
	if err := lockRecoveryOwner(ctx, db, tx, ownerTaskID); err != nil {
		return err
	}
	if err := validateRecoveryEnvironmentIdentity(ctx, db, tx, req, ownerTaskID); err != nil {
		return err
	}
	if err := validateRecoveryClaim(ctx, db, tx, req); err != nil {
		return err
	}
	return validateRecoveryRepositoryIdentity(ctx, db, tx, req)
}

func loadRecoveryEnvironmentOwner(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID string) (string, error) {
	var ownerTaskID string
	err := tx.QueryRowContext(ctx, db.Rebind(`SELECT task_id FROM task_environments WHERE id = ?`), environmentID).Scan(&ownerTaskID)
	return ownerTaskID, err
}

func lockRecoveryOwner(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, ownerTaskID string) error {
	lockQuery := `SELECT id FROM tasks WHERE id = ?`
	if strings.EqualFold(db.DriverName(), "pgx") || strings.EqualFold(db.DriverName(), "postgres") {
		lockQuery += ` FOR UPDATE`
	}
	var lockedID string
	return tx.QueryRowContext(ctx, db.Rebind(lockQuery), ownerTaskID).Scan(&lockedID)
}

func validateRecoveryEnvironmentIdentity(
	ctx context.Context,
	db *sqlx.DB,
	tx *sqlx.Tx,
	req Registration,
	ownerTaskID string,
) error {
	var currentOwner, executorType string
	var generation int64
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT task_id, ownership_generation, executor_type FROM task_environments WHERE id = ?
	`), req.TaskEnvironmentID).Scan(&currentOwner, &generation, &executorType); err != nil {
		return err
	}
	if currentOwner != req.OwnerTaskID || ownerTaskID != req.OwnerTaskID ||
		generation != req.OwnershipGeneration || executorType != req.ExecutorType {
		return ErrIdentityMismatch
	}
	return nil
}

func validateRecoveryClaim(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, req Registration) error {
	claim, err := recoveryclaim.GetTx(ctx, db, tx, req.TaskEnvironmentID)
	if err != nil {
		return err
	}
	if claim == nil || claim.OwnerTaskID != req.OwnerTaskID || claim.OwnershipGeneration != req.OwnershipGeneration ||
		claim.SessionID != req.SessionID || claim.OperationID != req.OperationID || claim.ExecutorType != req.ExecutorType {
		return ErrIdentityMismatch
	}
	return nil
}

func validateRecoveryRepositoryIdentity(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, req Registration) error {
	var identityRows int
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT COUNT(1) FROM task_environment_repos
		WHERE task_environment_id = ? AND repository_id = ?
		  AND ((worktree_id = ? AND worktree_path = ?)
		    OR (worktree_id = ? AND worktree_path = ?))
		  AND deleted_at IS NULL AND status NOT IN ('failed', 'deleted')
	`), req.TaskEnvironmentID, req.RepositoryID, req.WorktreeID, req.OriginalPath,
		req.ReplacementID, req.ReplacementPath).Scan(&identityRows); err != nil {
		return err
	}
	if identityRows != 1 {
		return ErrIdentityMismatch
	}
	return nil
}

func loadArtifactProofs(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, req Registration) ([]string, map[string]string, error) {
	var encodedPaths, encodedIdentities string
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT artifact_paths_json, artifact_identities_json FROM task_environment_recovery_artifacts
		WHERE task_environment_id = ? AND operation_id = ? AND worktree_id = ?
	`), req.TaskEnvironmentID, req.OperationID, req.WorktreeID).Scan(&encodedPaths, &encodedIdentities)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, make(map[string]string), nil
	}
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	if err := json.Unmarshal([]byte(encodedPaths), &paths); err != nil {
		return nil, nil, ErrIdentityMismatch
	}
	var identities map[string]string
	if err := json.Unmarshal([]byte(encodedIdentities), &identities); err != nil {
		return nil, nil, ErrIdentityMismatch
	}
	if identities == nil {
		identities = make(map[string]string)
	}
	if err := validateArtifactIdentities(paths, identities); err != nil {
		return nil, nil, err
	}
	var identityMatches int
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT COUNT(1) FROM task_environment_recovery_artifacts
		WHERE task_environment_id = ? AND operation_id = ? AND worktree_id = ?
		  AND owner_task_id = ? AND ownership_generation = ? AND session_id = ?
		  AND executor_type = ? AND repository_id = ? AND original_path = ?
		  AND replacement_id = ? AND replacement_path = ? AND layout_version = ?
		  AND provenance = ?
	`), req.TaskEnvironmentID, req.OperationID, req.WorktreeID, req.OwnerTaskID,
		req.OwnershipGeneration, req.SessionID, req.ExecutorType, req.RepositoryID,
		req.OriginalPath, req.ReplacementID, req.ReplacementPath, req.LayoutVersion,
		req.Provenance).Scan(&identityMatches); err != nil {
		return nil, nil, err
	}
	if identityMatches != 1 {
		return nil, nil, ErrIdentityMismatch
	}
	return paths, identities, nil
}

func containsPath(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}

func mergePaths(existing, addition []string) ([]string, error) {
	merged := make(map[string]struct{}, len(existing)+len(addition))
	for _, path := range append(existing, addition...) {
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
			return nil, ErrIdentityMismatch
		}
		merged[path] = struct{}{}
	}
	if len(merged) == 0 || len(merged) > maximumArtifactPathCount {
		return nil, ErrIdentityMismatch
	}
	paths := make([]string, 0, len(merged))
	for path := range merged {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	encoded, _ := json.Marshal(paths)
	if len(encoded) > maximumArtifactPathByteSize {
		return nil, fmt.Errorf("recovery artifact registration is too large")
	}
	return paths, nil
}

// ExclusionPaths returns exact registered paths that are lexically contained
// in the supplied task root. Private artifacts naturally produce no entries.
func ExclusionPaths(registered []Registered, taskRoot string) []string {
	root := filepath.Clean(taskRoot)
	set := make(map[string]struct{})
	for _, item := range registered {
		for _, path := range item.ArtifactPaths {
			registeredIdentity := item.ArtifactIdentities[path]
			if registeredIdentity == "" {
				continue
			}
			currentIdentity, ok := FilesystemIdentity(path)
			if !ok || currentIdentity != registeredIdentity {
				continue
			}
			clean := filepath.Clean(path)
			rel, err := filepath.Rel(root, clean)
			if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				set[clean] = struct{}{}
			}
		}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
