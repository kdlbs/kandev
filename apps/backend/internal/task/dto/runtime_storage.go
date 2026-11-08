package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Runtime storage advisory states. They describe the source observation, never
// a process or prompt outcome.
const (
	RuntimeStorageStateWarning     = "warning"
	RuntimeStorageStateNormal      = "normal"
	RuntimeStorageStateUnavailable = "unavailable"
)

// Runtime storage locations distinguish the backend host from a remote
// executor so the UI never offers local cleanup for a remote filesystem.
const (
	RuntimeStorageLocationLocal    = "local_runtime"
	RuntimeStorageLocationExecutor = "executor_runtime"
	RuntimeStorageLocationUnknown  = "unknown"
)

// RuntimeStorageHealthDTO is the optional live capacity advisory attached to
// one session. Measurements remain unknown when the source did not supply
// them.
type RuntimeStorageHealthDTO struct {
	State          string     `json:"state"`
	StorageKind    string     `json:"storage_kind,omitempty"`
	Location       string     `json:"location"`
	ExecutorLabel  string     `json:"executor_label,omitempty"`
	TotalBytes     *int64     `json:"total_bytes,omitempty"`
	AvailableBytes *int64     `json:"available_bytes,omitempty"`
	ObservedAt     *time.Time `json:"observed_at,omitempty"`
	// AgeSeconds is the backend receipt age. Browsers add local monotonic
	// elapsed time instead of comparing source and browser clocks.
	AgeSeconds     int64                     `json:"age_seconds"`
	WritePressure  bool                      `json:"write_pressure,omitempty"`
	ProducerPaused bool                      `json:"producer_paused,omitempty"`
	DeliveryState  string                    `json:"delivery_state,omitempty"`
	FailureStorage *models.AgentErrorStorage `json:"failure_storage,omitempty"`
	Revision       uint64                    `json:"revision"`
	Epoch          uint64                    `json:"epoch"`
}

// RuntimeStorageHealthProvider surfaces the lifecycle-owned observation
// without coupling serialization to the lifecycle package.
type RuntimeStorageHealthProvider interface {
	RuntimeStorageHealth(sessionID string) *RuntimeStorageHealthDTO
}

// EnrichRuntimeStorageHealth stamps the current observation onto a full
// session DTO. The provider is authoritative for both warning and normal
// states so a cleared advisory is visible after reload.
func EnrichRuntimeStorageHealth(session *TaskSessionDTO, provider RuntimeStorageHealthProvider) {
	if session == nil || provider == nil {
		return
	}
	session.RuntimeStorageHealth = provider.RuntimeStorageHealth(session.ID)
}
