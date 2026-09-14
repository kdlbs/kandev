package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

type adoptionDeliveryRepository struct {
	cursor        *models.AgentDeliveryCursor
	generation    *models.HarnessSessionGeneration
	generationErr error
	submissions   map[string]*models.AgentDeliverySubmission
	listErr       error
}

func (r *adoptionDeliveryRepository) ReceiveAgentDeliveryEvent(context.Context, *models.AgentDeliveryEvent, int64) (bool, error) {
	return true, nil
}

func (r *adoptionDeliveryRepository) GetAgentDeliveryCursor(context.Context, string) (*models.AgentDeliveryCursor, error) {
	if r.cursor == nil {
		return nil, sql.ErrNoRows
	}
	copy := *r.cursor
	return &copy, nil
}

func (r *adoptionDeliveryRepository) ProjectAgentDeliveryEvent(context.Context, *models.AgentDeliveryEvent, *models.AgentDeliveryEffect) (bool, error) {
	return true, nil
}

func (r *adoptionDeliveryRepository) GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error) {
	if r.generationErr != nil {
		return nil, r.generationErr
	}
	if r.generation == nil {
		return nil, models.ErrTaskSessionNotFound
	}
	copy := *r.generation
	return &copy, nil
}

func (r *adoptionDeliveryRepository) GetAgentDeliverySubmission(_ context.Context, id string) (*models.AgentDeliverySubmission, error) {
	submission := r.submissions[id]
	if submission == nil {
		return nil, errors.New("submission not found")
	}
	copy := *submission
	return &copy, nil
}

func (r *adoptionDeliveryRepository) ListAgentDeliverySubmissions(context.Context, string) ([]*models.AgentDeliverySubmission, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]*models.AgentDeliverySubmission, 0, len(r.submissions))
	for _, submission := range r.submissions {
		copy := *submission
		result = append(result, &copy)
	}
	return result, nil
}

func newAdoptionManager(repository AgentDeliveryRepository) *Manager {
	manager := &Manager{streamManager: NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)}
	manager.streamManager.setAgentDeliveryRepository(repository)
	return manager
}

func durableAdoptionStatus() *agentctl.DeliveryStatus {
	return &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		StreamID:          "stream-1",
		Stream: &journal.Stream{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", HighWater: 5, Acknowledged: 2, FirstRetained: 3,
		},
	}
}

func durableAdoptionRepository() *adoptionDeliveryRepository {
	return &adoptionDeliveryRepository{
		cursor: &models.AgentDeliveryCursor{
			SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 2,
			StreamID: "stream-1", ProjectedSequence: 2,
		},
		generation: &models.HarnessSessionGeneration{
			SessionID: "session-1", IncarnationID: "incarnation-1", Generation: 2,
			NativeSessionID: "native-1", OriginalWorkspace: "/original/workspace",
		},
		submissions: make(map[string]*models.AgentDeliverySubmission),
	}
}

func TestDurableAdoptionRestoresProjectedCursor(t *testing.T) {
	repository := durableAdoptionRepository()
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{SessionID: "session-1", WorkspacePath: "/current/workspace"}
	ri := &ExecutorInstance{
		ProviderSessionID: "native-1",
		DeliveryStatus:    durableAdoptionStatus(),
	}

	if err := manager.restoreRecoveredDelivery(context.Background(), execution, ri); err != nil {
		t.Fatalf("restoreRecoveredDelivery: %v", err)
	}
	if execution.DeliveryMode != DurableDeliveryV1 {
		t.Fatalf("delivery mode = %q, want %q", execution.DeliveryMode, DurableDeliveryV1)
	}
	if execution.DeliveryStreamID != "stream-1" || execution.DeliveryIncarnationID != "incarnation-1" || execution.DeliveryHarnessGeneration != 2 {
		t.Fatalf("delivery identity = %q/%q/%d", execution.DeliveryStreamID, execution.DeliveryIncarnationID, execution.DeliveryHarnessGeneration)
	}
	if execution.DeliveryReplayCursor != 2 {
		t.Fatalf("replay cursor = %d, want 2", execution.DeliveryReplayCursor)
	}
	if execution.OriginalWorkspacePath != "/original/workspace" {
		t.Fatalf("original workspace = %q", execution.OriginalWorkspacePath)
	}
}

func TestDurableAdoptionRestoresQuietSubmission(t *testing.T) {
	repository := durableAdoptionRepository()
	repository.submissions["submission-1"] = &models.AgentDeliverySubmission{
		ID: "submission-1", SessionID: "session-1", IncarnationID: "incarnation-1",
		HarnessGeneration: 2, State: models.DeliverySubmissionDispatching,
	}
	status := durableAdoptionStatus()
	status.Submissions = []journal.SubmissionSummary{{
		ID: "submission-1", SessionID: "session-1", IncarnationID: "incarnation-1",
		HarnessGeneration: 2, State: journal.SubmissionDispatching,
	}}
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{SessionID: "session-1"}

	if err := manager.restoreRecoveredDelivery(context.Background(), execution, &ExecutorInstance{DeliveryStatus: status}); err != nil {
		t.Fatalf("restoreRecoveredDelivery: %v", err)
	}
	if got := execution.deliverySubmissionIDSnapshot(); got != "submission-1" {
		t.Fatalf("submission identity = %q, want submission-1", got)
	}
}

func TestDurableAdoptionMatchesRetainedPeerTerminalToActiveBackendSubmission(t *testing.T) {
	repository := durableAdoptionRepository()
	repository.submissions["submission-terminal"] = &models.AgentDeliverySubmission{
		ID:                "submission-terminal",
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		PayloadHash:       "hash-terminal",
		State:             models.DeliverySubmissionDispatching,
	}
	status := durableAdoptionStatus()
	status.Stream.HighWater = 3
	status.Stream.Acknowledged = 2
	status.Stream.FirstRetained = 3
	status.Submissions = []journal.SubmissionSummary{{
		ID:                    "submission-terminal",
		SessionID:             "session-1",
		IncarnationID:         "incarnation-1",
		HarnessGeneration:     2,
		Hash:                  "hash-terminal",
		State:                 journal.SubmissionCompleted,
		TerminalEventRetained: true,
		TerminalSequence:      3,
	}}
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{SessionID: "session-1"}

	if err := manager.restoreRecoveredDelivery(context.Background(), execution, &ExecutorInstance{DeliveryStatus: status}); err != nil {
		t.Fatalf("restoreRecoveredDelivery: %v", err)
	}
	if got := execution.deliverySubmissionIDSnapshot(); got != "submission-terminal" {
		t.Fatalf("submission identity = %q, want submission-terminal", got)
	}
}

func TestDurableAdoptionLooksUpRetainedPeerTerminalOmittedFromSummary(t *testing.T) {
	repository := durableAdoptionRepository()
	repository.submissions["submission-terminal"] = &models.AgentDeliverySubmission{
		ID:                "submission-terminal",
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		PayloadHash:       "hash-terminal",
		State:             models.DeliverySubmissionDispatching,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agent/submissions/submission-terminal" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(journal.Submission{
			ID:                    "submission-terminal",
			SessionID:             "session-1",
			IncarnationID:         "incarnation-1",
			HarnessGeneration:     2,
			Hash:                  "hash-terminal",
			State:                 journal.SubmissionCompleted,
			TerminalEventRetained: true,
			TerminalSequence:      3,
		})
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}

	status := durableAdoptionStatus()
	status.Stream.HighWater = 3
	status.Stream.Acknowledged = 2
	status.Stream.FirstRetained = 3
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{SessionID: "session-1"}
	client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger())

	if err := manager.restoreRecoveredDelivery(context.Background(), execution, &ExecutorInstance{
		DeliveryStatus: status,
		Client:         client,
	}); err != nil {
		t.Fatalf("restoreRecoveredDelivery: %v", err)
	}
	if got := execution.deliverySubmissionIDSnapshot(); got != "submission-terminal" {
		t.Fatalf("submission identity = %q, want submission-terminal", got)
	}
}

func TestDurableAdoptionRestoresLifecycleInitialSubmissionWithoutBackendRow(t *testing.T) {
	repository := durableAdoptionRepository()
	status := durableAdoptionStatus()
	status.Submissions = []journal.SubmissionSummary{{
		ID:                initialPromptSubmissionID("session-1", 2),
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		State:             journal.SubmissionDispatching,
	}}
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{
		SessionID:                 "session-1",
		DeliveryHarnessGeneration: 2,
	}

	if err := manager.restoreRecoveredDelivery(context.Background(), execution, &ExecutorInstance{DeliveryStatus: status}); err != nil {
		t.Fatalf("restoreRecoveredDelivery: %v", err)
	}
	if got := execution.deliverySubmissionIDSnapshot(); got != initialPromptSubmissionID("session-1", 2) {
		t.Fatalf("submission identity = %q, want lifecycle initial submission", got)
	}
}

func TestDurableAdoptionRejectsUnknownPeerSubmissionWithoutBackendRow(t *testing.T) {
	repository := durableAdoptionRepository()
	status := durableAdoptionStatus()
	status.Submissions = []journal.SubmissionSummary{{
		ID:                "prompt:unknown",
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		State:             journal.SubmissionDispatching,
	}}
	manager := newAdoptionManager(repository)
	err := manager.restoreRecoveredDelivery(context.Background(), &AgentExecution{SessionID: "session-1"}, &ExecutorInstance{DeliveryStatus: status})
	if !errors.Is(err, ErrDurableAdoptionBlocked) {
		t.Fatalf("error = %v, want durable adoption blocked", err)
	}
}

func TestDurableAdoptionRejectsOwnerAndCursorEvidence(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*adoptionDeliveryRepository, *agentctl.DeliveryStatus)
		want    error
	}{
		{
			name: "owner mismatch",
			prepare: func(_ *adoptionDeliveryRepository, status *agentctl.DeliveryStatus) {
				status.HarnessGeneration = 3
			},
			want: ErrDurableAdoptionIdentityMismatch,
		},
		{
			name: "missing SQL owner",
			prepare: func(repository *adoptionDeliveryRepository, _ *agentctl.DeliveryStatus) {
				repository.generation = nil
			},
			want: ErrDurableAdoptionIdentityMismatch,
		},
		{
			name: "database cursor error",
			prepare: func(repository *adoptionDeliveryRepository, _ *agentctl.DeliveryStatus) {
				repository.cursor = nil
			},
			want: ErrDurableAdoptionEvidenceUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := durableAdoptionRepository()
			status := durableAdoptionStatus()
			test.prepare(repository, status)
			manager := newAdoptionManager(repository)
			err := manager.restoreRecoveredDelivery(context.Background(), &AgentExecution{SessionID: "session-1"}, &ExecutorInstance{DeliveryStatus: status})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestStandaloneDeliveryDiscoveryRequiresPositiveLegacyEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/delivery" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	executor := NewStandaloneExecutor(agentctl.NewControlClient(parsed.Hostname(), port, newTestLogger()), parsed.Hostname(), port, newTestLogger())
	executor.SetRecoveryRetryConfig(time.Second, 0)
	executor.SetPeerCapabilities([]string{"agent-survival.v1"})
	client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger())

	_, legacy, err := executor.discoverDeliveryStatus(context.Background(), client)
	if err == nil || legacy {
		t.Fatalf("discovery = legacy %v, err %v; 401 must block", legacy, err)
	}
}

func TestDurableAdoptionAllowsLegacyOnlyForUnsupportedOldRoute(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	executor := NewStandaloneExecutor(agentctl.NewControlClient(parsed.Hostname(), port, newTestLogger()), parsed.Hostname(), port, newTestLogger())
	executor.SetRecoveryRetryConfig(time.Second, 0)
	executor.SetPeerCapabilities([]string{"agent-survival.v1"})
	client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger())

	status, legacy, err := executor.discoverDeliveryStatus(context.Background(), client)
	if err != nil || !legacy || status != nil {
		t.Fatalf("discovery = status %#v, legacy %v, err %v; unsupported old route should be legacy", status, legacy, err)
	}
}

func TestDurableAdoptionLegacyBlocksUnresolvedSQLSubmission(t *testing.T) {
	repository := durableAdoptionRepository()
	repository.submissions["submission-1"] = &models.AgentDeliverySubmission{
		ID: "submission-1", SessionID: "session-1", State: models.DeliverySubmissionAccepted,
	}
	manager := newAdoptionManager(repository)
	err := manager.restoreRecoveredDelivery(context.Background(), &AgentExecution{SessionID: "session-1"}, &ExecutorInstance{DeliveryLegacyEvidence: true})
	if !errors.Is(err, ErrDurableAdoptionBlocked) {
		t.Fatalf("error = %v, want durable adoption blocked", err)
	}
}

func TestDurableAdoptionMissingCursorWithRetainedSuffixBlocks(t *testing.T) {
	repository := durableAdoptionRepository()
	repository.cursor = nil
	status := durableAdoptionStatus()
	manager := newAdoptionManager(repository)
	err := manager.restoreRecoveredDelivery(context.Background(), &AgentExecution{SessionID: "session-1"}, &ExecutorInstance{DeliveryStatus: status})
	if !errors.Is(err, ErrDurableAdoptionEvidenceUnavailable) {
		t.Fatalf("error = %v, want cursor evidence error", err)
	}
}

func TestDurableAdoptionErrorTextIncludesReason(t *testing.T) {
	err := fmt.Errorf("%w: %w", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	if !errors.Is(err, ErrDurableAdoptionIdentityMismatch) {
		t.Fatal("wrapped adoption reason was not discoverable")
	}
}
