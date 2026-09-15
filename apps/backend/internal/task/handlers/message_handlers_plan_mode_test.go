package handlers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestWSAddMessage_PlanModePersistsAndDispatches proves that a user message
// sent with plan_mode=true is recorded on the task and still enters the
// normal dispatch path. Plan mode changes the execution prompt; it does not
// turn message.add into a record-only operation.
func TestWSAddMessage_PlanModePersistsAndDispatches(t *testing.T) {
	tests := []struct {
		name            string
		planMode        bool
		wantMessageRows int
	}{
		{name: "plan_mode true persists and dispatches", planMode: true, wantMessageRows: 1},
		{name: "plan_mode false dispatches normally", planMode: false, wantMessageRows: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now().UTC()
			repo := &messageAddSwitchRepo{
				tasks: map[string]*models.Task{
					"t1": {ID: "t1", State: v1.TaskStateInProgress, UpdatedAt: now},
				},
				sessions: map[string]*models.TaskSession{
					"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, AgentProfileID: "profile-1", UpdatedAt: now},
				},
				primaryID: "s1",
			}
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			require.NoError(t, err)
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo,
			}, nil, log, service.RepositoryDiscoveryConfig{})
			orch := newPlanModeRecordingOrchestrator()
			h := NewMessageHandlers(svc, orch, log)
			payload := map[string]interface{}{
				"task_id": "t1", "session_id": "s1", "content": "plan-mode instruction",
				"plan_mode": tt.planMode,
			}
			req, err := ws.NewRequest("req-plan-mode", ws.ActionMessageAdd, payload)
			require.NoError(t, err)

			resp, err := h.wsAddMessage(t.Context(), req)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type)

			assert.Equal(t, tt.wantMessageRows, repo.messageCount(),
				"the user message must be persisted regardless of plan_mode")

			require.Eventually(t, func() bool { return orch.dispatchCalls() > 0 },
				time.Second, 5*time.Millisecond,
				"message.add must dispatch via SteerTask or PromptTask")
		})
	}
}

// planModeRecordingOrchestrator records every dispatch-path call so the
// plan_mode gate can be asserted precisely. SteerTask is the path a
// steer-eligible RUNNING session normally takes; PromptTask is the fallback
// and the path for non-steer-eligible sessions. StartCreatedSession covers
// newly created sessions.
type planModeRecordingOrchestrator struct {
	mu             sync.Mutex
	promptCalls    int32
	steerCalls     int32
	startCalls     int32
	queueCalls     int32
	turnStartCalls int32
}

func newPlanModeRecordingOrchestrator() *planModeRecordingOrchestrator {
	return &planModeRecordingOrchestrator{}
}

func (o *planModeRecordingOrchestrator) dispatchCalls() int32 {
	return atomic.LoadInt32(&o.promptCalls) + atomic.LoadInt32(&o.steerCalls) + atomic.LoadInt32(&o.startCalls)
}

func (o *planModeRecordingOrchestrator) PromptTask(_ context.Context, _, sessionID, _, _ string, _ bool, _ []v1.MessageAttachment, _ bool) (*orchestrator.PromptResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	atomic.AddInt32(&o.promptCalls, 1)
	_ = sessionID
	return &orchestrator.PromptResult{}, nil
}

func (o *planModeRecordingOrchestrator) SteerTask(_ context.Context, _, sessionID, _, _ string, _ bool, _ []v1.MessageAttachment) (*orchestrator.PromptResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	atomic.AddInt32(&o.steerCalls, 1)
	_ = sessionID
	return &orchestrator.PromptResult{}, nil
}

func (o *planModeRecordingOrchestrator) StartCreatedSession(_ context.Context, _, sessionID, _, _ string, _, _, _ bool, _ []v1.MessageAttachment, _ []v1.EntityReference) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	atomic.AddInt32(&o.startCalls, 1)
	_ = sessionID
	return nil
}

func (o *planModeRecordingOrchestrator) QueueUserPrompt(_ context.Context, _, _, _, _ string, _ bool, _ []v1.MessageAttachment, _ map[string]interface{}, _ bool) error {
	atomic.AddInt32(&o.queueCalls, 1)
	return nil
}

func (o *planModeRecordingOrchestrator) ProcessOnTurnStart(_ context.Context, _, sessionID string) (orchestrator.ProcessOnTurnStartResult, error) {
	atomic.AddInt32(&o.turnStartCalls, 1)
	_ = sessionID
	return orchestrator.ProcessOnTurnStartResult{}, nil
}

func (o *planModeRecordingOrchestrator) ResumeTaskSession(context.Context, string, string) error {
	return nil
}
func (*planModeRecordingOrchestrator) StepRequiresCompletionSignal(context.Context, string) bool {
	return false
}
func (*planModeRecordingOrchestrator) ForegroundActivity(string) v1.ForegroundActivity {
	return v1.ForegroundActivityGenerating
}
func (*planModeRecordingOrchestrator) SteerEligible(string, models.TaskSessionState) bool {
	return true
}
