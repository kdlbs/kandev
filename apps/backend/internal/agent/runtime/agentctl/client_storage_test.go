package client

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestDeliveryStorageHTTPErrorRetainsValidatedEvidence(t *testing.T) {
	body := []byte(`{"code":"STORAGE_FULL","message":"STORAGE_FULL","storage":{"version":1,"reason":"disk_space_low","observed_at":"2026-10-27T10:00:00Z","disk_available_bytes":1024,"disk_required_bytes":2048}}`)
	diagnostic := deliveryResponseStorage(body)
	if diagnostic == nil || diagnostic.Reason != streams.DeliveryStorageReasonDiskSpaceLow {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
	if diagnostic.DiskAvailableBytes == nil || *diagnostic.DiskAvailableBytes != 1024 {
		t.Fatalf("measurements = %+v", diagnostic)
	}
}

func TestDeliveryStorageHTTPErrorRejectsForeignEvidence(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"storage":{"version":99,"reason":"disk_space_low"}}`),
		[]byte(`{"storage":{"version":1,"reason":"provider_said_disk"}}`),
		[]byte(`{not json`),
		[]byte(`{"code":"STORAGE_FULL"}`),
	}
	for _, body := range cases {
		if diagnostic := deliveryResponseStorage(body); diagnostic != nil {
			t.Fatalf("foreign body produced a storage claim: %+v", diagnostic)
		}
	}
}

func TestDeliveryStoragePromptRejectionCarriesEvidence(t *testing.T) {
	diagnostic := &streams.DeliveryStorageDiagnostic{
		Version: 1, Reason: streams.DeliveryStorageReasonDiskSpaceLow, ObservedAt: time.Now().UTC(),
	}
	payload := ws.ErrorPayload{
		Code:    "CONFLICT",
		Message: "prompt submission could not be admitted",
		Details: map[string]interface{}{"storage": diagnostic},
	}
	parsed, ok := StorageDiagnosticFromError(promptStorageError(payload))
	if !ok || parsed.Reason != diagnostic.Reason {
		t.Fatalf("parsed = %+v, ok = %v", parsed, ok)
	}
	if promptStorageError(ws.ErrorPayload{Message: "no free space"}) != nil {
		t.Fatal("plain provider text produced storage evidence")
	}
}

func TestDeliveryStorageTypedErrorLookup(t *testing.T) {
	diagnostic := &streams.DeliveryStorageDiagnostic{Version: 1, Reason: streams.DeliveryStorageReasonFilesystemFull}
	err := &StorageDiagnosticError{Message: "storage full", Diagnostic: diagnostic}
	found, ok := StorageDiagnosticFromError(err)
	if !ok || found.Reason != streams.DeliveryStorageReasonFilesystemFull {
		t.Fatalf("found = %+v, ok = %v", found, ok)
	}
	if _, ok := StorageDiagnosticFromError(nil); ok {
		t.Fatal("nil error produced evidence")
	}
}
