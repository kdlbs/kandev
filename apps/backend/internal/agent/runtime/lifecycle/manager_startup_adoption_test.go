package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestReuseExistingAdoptsWithoutInitialize(t *testing.T) {
	peer := newMockAgentServer(t)
	status := durableAdoptionStatus()
	status.Stream.HighWater = 2
	status.Stream.FirstRetained = 1
	initialSubmissionID := initialPromptSubmissionID("session-1", 2)
	status.Submissions = []journal.SubmissionSummary{{
		ID: initialSubmissionID, SessionID: "session-1", IncarnationID: "incarnation-1",
		HarnessGeneration: 2, State: journal.SubmissionDispatching,
	}}
	peer.instanceID = "execution-1"
	peer.taskSessionID = "session-1"
	peer.nativeSessionID = "native-1"
	peer.deliveryStatus = status
	t.Cleanup(peer.Close)

	manager := newTestManager(t)
	manager.profileResolver = &restartProfileResolver{profile: &AgentProfileInfo{
		ProfileID: "profile-1", AgentID: "auggie", AgentName: "auggie", Model: "model-1",
	}}
	manager.SetAgentDeliveryRepository(durableAdoptionRepository())

	execution := &AgentExecution{
		ID:             "execution-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		AgentCommand:   "auggie",
		WorkspacePath:  "/workspace",
		ACPSessionID:   "native-1",
		metadata: map[string]interface{}{
			MetadataKeyReuseExistingProcess: true,
		},
		agentctl: createTestClient(t, peer.server.URL),
	}
	t.Cleanup(execution.agentctl.Close)
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("register execution: %v", err)
	}

	if err := manager.StartAgentProcess(context.Background(), execution.ID); err != nil {
		t.Fatalf("StartAgentProcess: %v", err)
	}
	if got := peer.getActionLog(); len(got) != 0 {
		t.Fatalf("ACP actions on existing agent = %v, want none", got)
	}
	if execution.ACPSessionID != "native-1" || !execution.isSessionInitialized() {
		t.Fatalf("reattached session = %q, initialized=%t", execution.ACPSessionID, execution.isSessionInitialized())
	}
	if execution.DeliveryMode != DurableDeliveryV1 || execution.DeliveryStreamID != "stream-1" {
		t.Fatalf("restored delivery = %q/%q, want durable stream-1", execution.DeliveryMode, execution.DeliveryStreamID)
	}
	if got := execution.deliverySubmissionIDSnapshot(); got != initialSubmissionID {
		t.Fatalf("active submission = %q, want preserved %q", got, initialSubmissionID)
	}
	if !execution.agentctl.HasAgentStream() {
		t.Fatal("updates stream was not attached to the existing agent")
	}
}

func TestReuseExistingIdentityMismatchPreservesPeer(t *testing.T) {
	peer := newMockAgentServer(t)
	status := durableAdoptionStatus()
	peer.instanceID = "execution-1"
	peer.taskSessionID = "session-1"
	peer.nativeSessionID = "different-native-session"
	peer.deliveryStatus = status
	t.Cleanup(peer.Close)

	manager := newTestManager(t)
	manager.SetAgentDeliveryRepository(durableAdoptionRepository())
	execution := &AgentExecution{
		ID:           "execution-1",
		SessionID:    "session-1",
		ACPSessionID: "native-1",
		AgentCommand: "auggie",
		metadata: map[string]interface{}{
			MetadataKeyReuseExistingProcess: true,
		},
		agentctl: createTestClient(t, peer.server.URL),
	}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("register execution: %v", err)
	}
	if err := manager.StartAgentProcess(context.Background(), execution.ID); !errors.Is(err, ErrAgentReattachment) {
		t.Fatalf("StartAgentProcess error = %v, want ErrAgentReattachment", err)
	}
	response, err := http.Get(peer.server.URL + "/health") //nolint:noctx // test-only live-peer assertion
	if err != nil {
		t.Fatalf("live peer health request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live peer status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := peer.getActionLog(); len(got) != 0 {
		t.Fatalf("ACP actions on identity mismatch = %v, want none", got)
	}
	if execution.agentctl.HasAgentStream() {
		t.Fatal("failed reattachment unexpectedly attached a stream")
	}
}
