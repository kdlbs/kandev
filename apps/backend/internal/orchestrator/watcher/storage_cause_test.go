package watcher

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryStorageCauseProjection(t *testing.T) {
	observedAt := time.Now().UTC()
	available, required := int64(1024), int64(2048)
	data := withDeliveryStorageCause(AgentEventData{
		SessionID: "session",
		Storage: &models.AgentErrorStorage{
			Reason:             models.AgentStorageReasonDiskSpaceLow,
			Location:           "local_runtime",
			ObservedAt:         &observedAt,
			DiskAvailableBytes: &available,
			DiskRequiredBytes:  &required,
		},
	})
	if len(data.Causes) != 1 {
		t.Fatalf("causes = %+v", data.Causes)
	}
	cause := data.Causes[0]
	if cause.Operation != models.AgentErrorCauseOperationDelivery ||
		cause.Code != models.AgentErrorCauseCodeDeliveryStorage {
		t.Fatalf("cause identity = %+v", cause)
	}
	if cause.Storage == nil || cause.Storage.Reason != models.AgentStorageReasonDiskSpaceLow {
		t.Fatalf("cause storage = %+v", cause.Storage)
	}
}

func TestDeliveryStorageCauseProjectionIgnoresPlainFailures(t *testing.T) {
	data := withDeliveryStorageCause(AgentEventData{SessionID: "session", ErrorMessage: "no space"})
	if len(data.Causes) != 0 {
		t.Fatalf("provider text became storage evidence: %+v", data.Causes)
	}
}
