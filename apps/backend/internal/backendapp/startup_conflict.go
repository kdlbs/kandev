package backendapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/common/config"
)

const (
	startupConflictMarkerPrefix      = "KANDEV_DESKTOP_CONFLICT_V1 "
	maxStartupConflictPathBytes      = 4096
	maxStartupConflictOwnerFieldSize = 256
	maxStartupConflictMarkerBytes    = 16 * 1024
	startupConflictStorageUnknown    = "unknown"
	startupConflictSQLiteInHome      = "sqlite_in_home"
)

type desktopStartupConflictMarker struct {
	Version      int                          `json:"version"`
	TargetKind   ownershiplock.TargetKind     `json:"target_kind"`
	TargetPath   string                       `json:"target_path"`
	StorageKind  string                       `json:"storage_kind"`
	DatabasePath string                       `json:"database_path,omitempty"`
	Owner        *desktopStartupConflictOwner `json:"owner,omitempty"`
}

type desktopStartupConflictOwner struct {
	PID        int64  `json:"pid,omitempty"`
	Executable string `json:"executable,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
}

func writeDesktopStartupConflictMarker(w io.Writer, cfg *config.Config, err error) bool {
	var conflict *ownershiplock.ConflictError
	if !errors.As(err, &conflict) || w == nil || conflict == nil {
		return false
	}
	if (conflict.Target.Kind != ownershiplock.TargetHome && conflict.Target.Kind != ownershiplock.TargetDatabase) ||
		!validStartupConflictPath(conflict.Target.ResourcePath) {
		return false
	}

	storageKind, databasePath := startupConflictStorage(cfg, conflict)
	marker := desktopStartupConflictMarker{
		Version:      1,
		TargetKind:   conflict.Target.Kind,
		TargetPath:   conflict.Target.ResourcePath,
		StorageKind:  storageKind,
		DatabasePath: databasePath,
		Owner:        startupConflictOwnerDetails(conflict.Owner),
	}
	payload, marshalErr := json.Marshal(marker)
	if marshalErr != nil || len(startupConflictMarkerPrefix)+len(payload) > maxStartupConflictMarkerBytes {
		return false
	}
	_, writeErr := fmt.Fprintf(w, "%s%s\n", startupConflictMarkerPrefix, payload)
	return writeErr == nil
}

func startupConflictStorage(cfg *config.Config, conflict *ownershiplock.ConflictError) (string, string) {
	if cfg == nil {
		return startupConflictStorageUnknown, ""
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Database.Driver)) {
	case "sqlite":
		targets, err := ownershiplock.Targets(cfg.ResolvedHomeDir(), cfg.Database.Driver, cfg.Database.Path)
		if err != nil {
			return startupConflictStorageUnknown, ""
		}
		for _, target := range targets {
			if target.Kind == ownershiplock.TargetDatabase {
				return "sqlite_external", target.ResourcePath
			}
		}
		if conflict.Target.Kind == ownershiplock.TargetHome {
			if strings.TrimSpace(cfg.Database.Path) == "" {
				return startupConflictSQLiteInHome, filepath.Join(conflict.Target.ResourcePath, "data", "kandev.db")
			}
			path := strings.TrimSpace(cfg.Database.Path)
			if absolute, err := filepath.Abs(path); err == nil {
				path = filepath.Clean(absolute)
			}
			return startupConflictSQLiteInHome, path
		}
		return startupConflictSQLiteInHome, conflict.Target.ResourcePath
	case "postgres":
		return "postgres", ""
	default:
		return startupConflictStorageUnknown, ""
	}
}

func startupConflictOwnerDetails(owner *ownershiplock.OwnerRecord) *desktopStartupConflictOwner {
	if owner == nil {
		return nil
	}
	details := &desktopStartupConflictOwner{PID: owner.PID}
	details.Executable = sanitizeStartupConflictOwnerField(owner.Executable)
	startedAt := sanitizeStartupConflictOwnerField(owner.StartedAt)
	if startedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, startedAt); err == nil {
			details.StartedAt = startedAt
		}
	}
	if details.PID <= 0 && details.Executable == "" && details.StartedAt == "" {
		return nil
	}
	return details
}

func sanitizeStartupConflictOwnerField(value string) string {
	value = strings.ToValidUTF8(value, "�")
	var result strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) {
			continue
		}
		if result.Len()+utf8.RuneLen(character) > maxStartupConflictOwnerFieldSize {
			break
		}
		result.WriteRune(character)
	}
	return strings.TrimSpace(result.String())
}

func validStartupConflictPath(path string) bool {
	return path != "" && len(path) <= maxStartupConflictPathBytes && utf8.ValidString(path)
}
