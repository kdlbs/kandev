package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestDeliveryStorageCauseSurvivesDrainedTerminal(t *testing.T) {
	diagnostic := &streams.DeliveryStorageDiagnostic{
		Version: streams.DeliveryStorageDiagnosticVersion,
		Reason:  streams.DeliveryStorageReasonDiskSpaceLow,
	}
	manager := &Manager{deliveryHealth: DeliveryHealth{
		Failure: &agenttypes.DeliveryFailureSnapshot{
			SubmissionID: "submission-1",
			Storage:      diagnostic,
		},
	}}
	update := manager.attachDeliveryFailureStorage(adapter.AgentEvent{
		Type: streams.EventTypeComplete, DeliverySubmissionID: "submission-1",
	})
	if update.Storage == nil || update.Storage.Reason != streams.DeliveryStorageReasonDiskSpaceLow {
		t.Fatalf("terminal storage = %+v", update.Storage)
	}
	other := manager.attachDeliveryFailureStorage(adapter.AgentEvent{
		Type: streams.EventTypeComplete, DeliverySubmissionID: "submission-2",
	})
	if other.Storage != nil {
		t.Fatalf("unrelated terminal inherited storage: %+v", other.Storage)
	}
}
