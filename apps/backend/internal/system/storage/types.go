package storage

import (
	"errors"
	"fmt"
	"path/filepath"
)

var ErrValidation = errors.New("storage maintenance settings validation")

var (
	ErrAdoptionRequired            = errors.New("go cache adoption requires the dedicated endpoint")
	ErrDedicatedDockerConfirmation = errors.New("dedicated Docker acknowledgement requires confirmation")
	ErrInvalidPersistedSettings    = errors.New("invalid persisted storage maintenance settings")
)

const (
	MinCheckIntervalHours       = 1
	MaxCheckIntervalHours       = 168
	MinIdleForMinutes           = 1
	MaxIdleForMinutes           = 1440
	MinGraceHours               = 24
	MaxGraceHours               = 2160
	MaxDockerUnusedHours        = 2562047
	MinCacheBytes         int64 = 1073741824
)

type ResourceSettings struct {
	Enabled bool `json:"enabled"`
}

type GoCacheSettings struct {
	Enabled     bool   `json:"enabled"`
	MaxBytes    int64  `json:"max_bytes"`
	AdoptedPath string `json:"adopted_path"`
}

type DockerSettings struct {
	DedicatedDaemonAcknowledged bool  `json:"dedicated_daemon_acknowledged"`
	BuildCacheEnabled           bool  `json:"build_cache_enabled"`
	BuildCacheKeepBytes         int64 `json:"build_cache_keep_bytes"`
	BuildCacheUnusedHours       int   `json:"build_cache_unused_hours"`
	UnusedImagesEnabled         bool  `json:"unused_images_enabled"`
	UnusedImagesHours           int   `json:"unused_images_hours"`
}

type StorageMaintenanceSettings struct {
	Enabled                  bool             `json:"enabled"`
	CheckIntervalHours       int              `json:"check_interval_hours"`
	IdleForMinutes           int              `json:"idle_for_minutes"`
	OrphanGraceHours         int              `json:"orphan_grace_hours"`
	QuarantineRetentionHours int              `json:"quarantine_retention_hours"`
	Workspaces               ResourceSettings `json:"workspaces"`
	KandevContainers         ResourceSettings `json:"kandev_containers"`
	GoCache                  GoCacheSettings  `json:"go_cache"`
	Docker                   DockerSettings   `json:"docker"`
}

func DefaultSettings() StorageMaintenanceSettings {
	return StorageMaintenanceSettings{
		CheckIntervalHours:       24,
		IdleForMinutes:           10,
		OrphanGraceHours:         168,
		QuarantineRetentionHours: 168,
		Workspaces:               ResourceSettings{Enabled: true},
		KandevContainers:         ResourceSettings{Enabled: true},
		GoCache: GoCacheSettings{
			MaxBytes: 16106127360,
		},
		Docker: DockerSettings{
			BuildCacheKeepBytes:   10737418240,
			BuildCacheUnusedHours: 168,
			UnusedImagesHours:     168,
		},
	}
}

func NormalizeSettings(in StorageMaintenanceSettings) (StorageMaintenanceSettings, error) {
	if err := validateRange("check_interval_hours", in.CheckIntervalHours, MinCheckIntervalHours, MaxCheckIntervalHours); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if err := validateRange("idle_for_minutes", in.IdleForMinutes, MinIdleForMinutes, MaxIdleForMinutes); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if err := validateRange("orphan_grace_hours", in.OrphanGraceHours, MinGraceHours, MaxGraceHours); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if err := validateRange("quarantine_retention_hours", in.QuarantineRetentionHours, MinGraceHours, MaxGraceHours); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if in.GoCache.MaxBytes < MinCacheBytes {
		return StorageMaintenanceSettings{}, validationError("go_cache.max_bytes must be at least %d", MinCacheBytes)
	}
	if in.Docker.BuildCacheKeepBytes < MinCacheBytes {
		return StorageMaintenanceSettings{}, validationError("docker.build_cache_keep_bytes must be at least %d", MinCacheBytes)
	}
	if err := validateRange(
		"docker.build_cache_unused_hours",
		in.Docker.BuildCacheUnusedHours,
		MinGraceHours,
		MaxDockerUnusedHours,
	); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if err := validateRange(
		"docker.unused_images_hours",
		in.Docker.UnusedImagesHours,
		MinGraceHours,
		MaxDockerUnusedHours,
	); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if (in.Docker.BuildCacheEnabled || in.Docker.UnusedImagesEnabled) && !in.Docker.DedicatedDaemonAcknowledged {
		return StorageMaintenanceSettings{}, validationError("docker cleanup requires dedicated daemon acknowledgement")
	}
	if in.GoCache.AdoptedPath != "" {
		if !filepath.IsAbs(in.GoCache.AdoptedPath) {
			return StorageMaintenanceSettings{}, validationError("go_cache.adopted_path must be absolute")
		}
		in.GoCache.AdoptedPath = filepath.Clean(in.GoCache.AdoptedPath)
	}
	return in, nil
}

func validateRange(field string, value, minValue, maxValue int) error {
	if value < minValue || value > maxValue {
		return validationError("%s must be between %d and %d", field, minValue, maxValue)
	}
	return nil
}

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}
