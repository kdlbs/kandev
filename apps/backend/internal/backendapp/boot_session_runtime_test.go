package backendapp

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/task/dto"
	userdto "github.com/kandev/kandev/internal/user/dto"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestBootTaskDetailSiblingRuntimeProjectionWiring(t *testing.T) {
	fixture := newJourneyReadFixture(t, 1)
	service := orchestrator.NewService(orchestrator.DefaultServiceConfig(), bus.NewMemoryEventBus(fixture.params.log), nil,
		&taskRepositoryAdapter{repo: fixture.taskRepo, svc: fixture.params.taskSvc}, fixture.taskRepo, nil, nil, nil, fixture.params.log)
	fixture.params.orchestratorSvc = service
	t.Cleanup(func() { _ = service.Stop() })
	selected := &models.TaskSession{ID: "selected", TaskID: fixture.taskIDs[0], State: models.TaskSessionStateRunning}
	sibling := &models.TaskSession{ID: "sibling", TaskID: fixture.taskIDs[0], State: models.TaskSessionStateRunning}
	task, err := fixture.params.taskSvc.GetTask(t.Context(), selected.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	builder := bootStateBuilder{p: fixture.params}
	state := builder.taskDetailInitialState(t.Context(), task, dto.FromTask(task), []*models.TaskSession{selected, sibling}, selected, selected.ID, userdto.UserSettingsResponse{}, false)
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		TaskSessions struct {
			Items map[string]struct {
				ForegroundActivity v1.ForegroundActivity `json:"foreground_activity"`
				ParkedEpoch        uint64                `json:"parked_epoch"`
			}
		}
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{selected.ID, sibling.ID} {
		item := response.TaskSessions.Items[id]
		if item.ForegroundActivity != v1.ForegroundActivityGenerating || item.ParkedEpoch != service.ParkedEpoch() || item.ParkedEpoch == 0 {
			t.Errorf("session %s lost runtime projections: %+v", id, item)
		}
	}
}

type bootSessionRuntimeValue struct {
	activity       v1.ForegroundActivity
	cancelling     bool
	cancelRevision uint64
	parked         bool
	parkedRevision uint64
}

type bootSessionRuntimeFixture map[string]bootSessionRuntimeValue

func (p bootSessionRuntimeFixture) ForegroundActivity(id string) v1.ForegroundActivity {
	return p[id].activity
}
func (p bootSessionRuntimeFixture) CancellationPending(id string) bool { return p[id].cancelling }
func (p bootSessionRuntimeFixture) CancellationPendingSnapshot(id string) (bool, uint64) {
	return p[id].cancelling, p[id].cancelRevision
}
func (p bootSessionRuntimeFixture) ParkedSnapshot(id string) (bool, uint64) {
	return p[id].parked, p[id].parkedRevision
}
func (p bootSessionRuntimeFixture) ParkedEpoch() uint64 { return 42 }

func TestBootTaskDetailCompactRuntimeStates(t *testing.T) {
	fixture := newJourneyReadFixture(t, 1)
	provider := bootSessionRuntimeFixture{
		"selected":   {v1.ForegroundActivityGenerating, true, 7, false, 5},
		"background": {v1.ForegroundActivityBackground, true, 13, false, 8},
		"parked":     {v1.ForegroundActivityBackground, false, 20, true, 9},
		"settled":    {v1.ForegroundActivityGenerating, false, 24, false, 12},
	}
	sessions := []*models.TaskSession{}
	richSentinel := strings.Repeat("private rich metadata must stay selected-only ", 6000)
	for _, id := range []string{"selected", "background", "parked", "settled"} {
		state := models.TaskSessionStateRunning
		if id == "parked" || id == "settled" {
			state = models.TaskSessionStateWaitingForInput
		}
		sessions = append(sessions, &models.TaskSession{ID: id, TaskID: fixture.taskIDs[0], State: state,
			Metadata: map[string]any{"rich_sibling_state": richSentinel}, AgentProfileSnapshot: map[string]any{"rich": richSentinel},
		})
	}
	// The selected projection remains rich. Its data is deliberately small so
	// the total bound detects metadata leaking from any inactive sibling.
	sessions[0].Metadata = map[string]any{}
	sessions[0].AgentProfileSnapshot = nil
	for _, session := range sessions {
		if err := fixture.taskRepo.CreateTaskSession(t.Context(), session); err != nil {
			t.Fatal(err)
		}
	}
	observations, err := fixture.params.taskSvc.ListTaskSessionSummaryObservations(t.Context(), fixture.taskIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	compactSessions := make([]*models.TaskSession, 0, len(observations))
	for _, observation := range observations {
		compactSessions = append(compactSessions, observation.ToTaskSession())
	}
	state := map[string]any{}
	builder := bootStateBuilder{p: fixture.params}
	builder.addTaskDetailSessionsState(t.Context(), state, fixture.taskIDs[0], compactSessions, sessions[0], sessions[0].ID, provider)
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 32*1024 || strings.Contains(string(raw), "rich_sibling_state") {
		t.Fatalf("compact siblings leaked rich metadata: payload bytes=%d", len(raw))
	}
	var response struct {
		TaskSessions       struct{ Items map[string]dto.TaskSessionDTO }
		TaskSessionsByTask struct {
			ItemsByTaskID map[string][]dto.TaskSessionDTO
		}
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	list := response.TaskSessionsByTask.ItemsByTaskID[fixture.taskIDs[0]]
	if len(list) != len(sessions) {
		t.Fatalf("session membership changed: %+v", list)
	}
	for _, row := range list {
		want := provider[row.ID]
		expectedActivity := want.activity
		if row.ID == "settled" {
			expectedActivity = ""
		}
		if row.ForegroundActivity != expectedActivity || row.CancellationPending != want.cancelling || row.CancellationRevision != want.cancelRevision ||
			row.ParkedOnBackgroundWork != want.parked || row.Revision != want.parkedRevision || row.ParkedEpoch != 42 {
			t.Errorf("boot runtime row %s = %+v, want %+v epoch42", row.ID, row, want)
		}
		item := response.TaskSessions.Items[row.ID]
		if item.ForegroundActivity != row.ForegroundActivity || item.CancellationPending != row.CancellationPending || item.CancellationRevision != row.CancellationRevision ||
			item.ParkedOnBackgroundWork != row.ParkedOnBackgroundWork || item.Revision != row.Revision || item.ParkedEpoch != row.ParkedEpoch {
			t.Errorf("boot normalized/list projections diverged for %s", row.ID)
		}
	}
}
