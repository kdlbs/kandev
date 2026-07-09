package workspacescope

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
)

// FallbackWorkspaceID is used only in isolated tests or malformed databases
// where integration config exists before the workspaces table is available.
const FallbackWorkspaceID = "default"

// DefaultResolver resolves the default workspace used by legacy integration
// methods that predate explicit workspace IDs. New request paths should pass a
// workspace ID and bypass this fallback entirely.
//
// The resolved workspace is derived from the user's active workspace setting,
// which is mutable at runtime, so it is re-read on every call rather than
// memoized — caching it would pin the process to whichever workspace happened
// to be active first and hide a configured integration after the user switches
// workspaces.
type DefaultResolver struct{}

func (DefaultResolver) Resolve(db *sqlx.DB) (string, error) {
	return ResolveMigrationTarget(db)
}

// ResolveMigrationTarget returns the workspace that should receive an upgraded
// singleton integration config. It prefers the workspace stored in user
// settings, then the first workspace by creation time.
func ResolveMigrationTarget(db *sqlx.DB) (string, error) {
	if db == nil {
		return FallbackWorkspaceID, nil
	}
	hasWorkspaces, err := tableExists(db, "workspaces")
	if err != nil || !hasWorkspaces {
		return FallbackWorkspaceID, err
	}
	if workspaceID, err := activeWorkspaceFromUsers(db); err != nil {
		return "", err
	} else if workspaceID != "" {
		return workspaceID, nil
	}
	var id string
	err = db.Get(&id, `SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return FallbackWorkspaceID, nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func tableExists(db *sqlx.DB, name string) (bool, error) {
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name); err != nil {
		return false, err
	}
	return count > 0, nil
}

func activeWorkspaceFromUsers(db *sqlx.DB) (string, error) {
	hasUsers, err := tableExists(db, "users")
	if err != nil || !hasUsers {
		return "", err
	}
	var settings []string
	if err := db.Select(&settings, `SELECT settings FROM users ORDER BY updated_at DESC`); err != nil {
		return "", err
	}
	for _, raw := range settings {
		var parsed struct {
			WorkspaceID string `json:"workspace_id"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil || parsed.WorkspaceID == "" {
			continue
		}
		if ok, err := workspaceExists(db, parsed.WorkspaceID); err != nil {
			return "", err
		} else if ok {
			return parsed.WorkspaceID, nil
		}
	}
	return "", nil
}

func workspaceExists(db *sqlx.DB, id string) (bool, error) {
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM workspaces WHERE id = ?`, id); err != nil {
		return false, err
	}
	return count > 0, nil
}
