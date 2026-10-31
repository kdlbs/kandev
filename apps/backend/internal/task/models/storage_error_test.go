package models

import (
	"reflect"
	"testing"
	"time"
)

func int64Pointer(value int64) *int64 { return &value }

func TestDeliveryStorageCauseNormalizesBoundedEvidence(t *testing.T) {
	observedAt := time.Now().UTC()
	causes := NormalizeAgentErrorCauses([]AgentErrorCause{{
		Operation: AgentErrorCauseOperationDelivery,
		Code:      AgentErrorCauseCodeDeliveryStorage,
		Storage: &AgentErrorStorage{
			Reason:             AgentStorageReasonDiskSpaceLow,
			Location:           "local_runtime",
			ExecutorLabel:      " worktree ",
			ObservedAt:         &observedAt,
			DiskAvailableBytes: int64Pointer(1024),
			DiskRequiredBytes:  int64Pointer(-1),
			LimitBytes:         int64Pointer(0),
		},
	}})
	if len(causes) != 1 || causes[0].Storage == nil {
		t.Fatalf("causes = %+v", causes)
	}
	storage := causes[0].Storage
	if storage.Reason != AgentStorageReasonDiskSpaceLow || storage.Location != "local_runtime" {
		t.Fatalf("storage identity = %+v", storage)
	}
	if storage.ExecutorLabel != "worktree" {
		t.Fatalf("label = %q", storage.ExecutorLabel)
	}
	if storage.DiskAvailableBytes == nil || *storage.DiskAvailableBytes != 1024 {
		t.Fatalf("available = %v", storage.DiskAvailableBytes)
	}
	if storage.DiskRequiredBytes != nil || storage.LimitBytes != nil {
		t.Fatalf("malformed measurements survived: %+v", storage)
	}
}

func TestDeliveryStorageCauseRejectsForeignReasons(t *testing.T) {
	causes := NormalizeAgentErrorCauses([]AgentErrorCause{{
		Operation: AgentErrorCauseOperationDelivery,
		Code:      AgentErrorCauseCodeDeliveryStorage,
		Storage:   &AgentErrorStorage{Reason: "provider_said_disk", Location: "local_runtime"},
	}})
	if len(causes) != 1 || causes[0].Storage != nil {
		t.Fatalf("foreign reason survived: %+v", causes)
	}
	dropped := NormalizeAgentErrorCauses([]AgentErrorCause{{
		Operation: AgentErrorCauseOperationStart,
		Code:      AgentErrorCauseCodeDeliveryStorage,
		Storage:   &AgentErrorStorage{Reason: AgentStorageReasonDiskSpaceLow},
	}})
	if dropped[0].Storage != nil {
		t.Fatalf("storage evidence attached to a foreign operation: %+v", dropped)
	}
}

func TestDeliveryStorageCauseEqualityAndPersistence(t *testing.T) {
	observedAt := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	makeCause := func(available int64) AgentErrorCause {
		return AgentErrorCause{
			Operation: AgentErrorCauseOperationDelivery,
			Code:      AgentErrorCauseCodeDeliveryStorage,
			Storage: &AgentErrorStorage{
				Reason:             AgentStorageReasonFilesystemFull,
				Location:           "executor_runtime",
				ExecutorLabel:      "sprite",
				ObservedAt:         &observedAt,
				DiskAvailableBytes: int64Pointer(available),
			},
		}
	}
	left := NormalizeAgentErrorCauses([]AgentErrorCause{makeCause(10)})
	right := NormalizeAgentErrorCauses([]AgentErrorCause{makeCause(10)})
	if !AgentErrorCausesEqual(left, right) {
		t.Fatal("identical storage causes were not equal")
	}
	if AgentErrorCausesEqual(left, NormalizeAgentErrorCauses([]AgentErrorCause{makeCause(11)})) {
		t.Fatal("different storage measurements compared equal")
	}
	metadata := map[string]interface{}{
		SessionMetaKeyLastAgentError: map[string]interface{}{
			"message":     "Session interrupted",
			"occurred_at": "2026-10-27T10:00:00Z",
			"causes":      left,
		},
	}
	loaded, ok := LoadLastAgentError(metadata)
	if !ok || len(loaded.Causes) != 1 || loaded.Causes[0].Storage == nil {
		t.Fatalf("loaded error = %+v, ok = %v", loaded, ok)
	}
	if !reflect.DeepEqual(loaded.Causes[0].Storage, left[0].Storage) {
		t.Fatalf("storage = %+v, want %+v", loaded.Causes[0].Storage, left[0].Storage)
	}
}
