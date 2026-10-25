package service_test

import (
	"context"
	"encoding/json"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type engineRunsServiceAdapter struct {
	service *runsservice.Service
}

func (a engineRunsServiceAdapter) QueueRun(
	ctx context.Context, req engine.QueueRunRequest,
) (engine.QueueOutcome, error) {
	outcome, err := a.service.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: req.AgentProfileID,
		TaskID:         req.TaskID,
		WorkflowStepID: req.WorkflowStepID,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		Payload:        req.Payload,
		WakeWaveKey:    req.WaveKey,
		WakeWaveString: req.WaveString,
	})
	return engine.QueueOutcome(outcome), err
}

type crossTaskStepResolver struct{}

func (crossTaskStepResolver) WorkflowStepIDForTask(context.Context, string) (string, error) {
	return "step-target", nil
}

func childrenCompletedQueueInput(taskID, operationID string) engine.ActionInput {
	return engine.ActionInput{
		Trigger: engine.TriggerOnChildrenCompleted,
		State: engine.MachineState{
			TaskID:     "parent-1",
			WorkflowID: "workflow-1",
		},
		Step: engine.StepSpec{ID: "step-parent"},
		Action: engine.Action{
			Kind: engine.ActionQueueRun,
			QueueRun: &engine.QueueRunAction{
				Target:  "agent_profile_id:agent-a",
				TaskID:  taskID,
				Reason:  "task_children_completed",
				Payload: map[string]any{"marker": taskID},
			},
		},
		OperationID: operationID,
		Payload: engine.OnChildrenCompletedPayload{
			WaveKey:    "task_children_completed:parent-1:deadbeef",
			WaveString: "parent-1|child-1,child-2",
		},
	}
}

func TestQueueRunCallback_CrossTaskWaveIdentitySurvivesInEitherActionOrder(t *testing.T) {
	orders := []struct {
		name    string
		reverse bool
	}{
		{name: "same-task-first"},
		{name: "cross-task-first", reverse: true},
	}

	for _, tc := range orders {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := newTestServiceWithRepo(t)
			callback := engine.QueueRunCallback{
				Adapter:   engineRunsServiceAdapter{service: svc},
				TaskSteps: crossTaskStepResolver{},
			}
			actions := []engine.ActionInput{
				childrenCompletedQueueInput("this", "same-task"),
				childrenCompletedQueueInput("task-2", "cross-task"),
			}
			if tc.reverse {
				actions[0], actions[1] = actions[1], actions[0]
			}
			for _, action := range actions {
				if _, err := callback.Execute(t.Context(), action); err != nil {
					t.Fatalf("queue %s action: %v", action.OperationID, err)
				}
			}

			var rows []struct {
				Payload     string `db:"payload"`
				WakeWaveKey string `db:"wake_wave_key"`
			}
			if err := repo.Reader().SelectContext(t.Context(), &rows,
				`SELECT payload, wake_wave_key FROM runs WHERE agent_profile_id = ? AND reason = ?`,
				"agent-a", "task_children_completed"); err != nil {
				t.Fatalf("list queued runs: %v", err)
			}
			if len(rows) != 2 {
				t.Fatalf("queued runs = %d, want 2 (same-task and cross-task actions must both survive)", len(rows))
			}

			waveByTask := make(map[string]string, len(rows))
			for _, row := range rows {
				var payload map[string]any
				if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
					t.Fatalf("decode run payload %q: %v", row.Payload, err)
				}
				taskID, ok := payload["task_id"].(string)
				if !ok {
					t.Fatalf("run payload task_id = %v, want string", payload["task_id"])
				}
				waveByTask[taskID] = row.WakeWaveKey
			}
			if waveByTask["parent-1"] != "task_children_completed:parent-1:deadbeef" {
				t.Fatalf("parent wave key = %q, want source wave identity", waveByTask["parent-1"])
			}
			if waveByTask["task-2"] != "" {
				t.Fatalf("cross-task wave key = %q, want empty", waveByTask["task-2"])
			}
		})
	}
}
