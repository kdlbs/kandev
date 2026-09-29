package recoveryclaim

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskEnvironmentAdmissionClassification(t *testing.T) {
	waiting := models.TaskEnvironmentAdmissionConsumer{
		SessionID: "session-preserved", SessionState: models.TaskSessionStateWaitingForInput,
	}
	stopped := waiting
	stopped.HasExecutor = true
	stopped.ExecutorStatus = models.ExecutorRunningStatusStopped

	tests := []struct {
		name     string
		snapshot models.TaskEnvironmentAdmissionSnapshot
		want     models.TaskEnvironmentAdmissionClass
	}{
		{name: "no consumers", want: models.TaskEnvironmentAdmissionInactivePreserved},
		{name: "waiting without executor", snapshot: snapshotWith(waiting), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "waiting with stopped executor", snapshot: snapshotWith(stopped), want: models.TaskEnvironmentAdmissionInactivePreserved},
		{name: "pre-launch requester", snapshot: snapshotWith(models.TaskEnvironmentAdmissionConsumer{
			SessionID: "session-requester", SessionState: models.TaskSessionStateCreated, IsRequester: true,
		}), want: models.TaskEnvironmentAdmissionInactivePreserved},
		{name: "idle without executor", snapshot: snapshotWith(admissionConsumer(models.TaskSessionStateIdle)), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "terminal without executor", snapshot: snapshotWith(admissionConsumer(models.TaskSessionStateCompleted)), want: models.TaskEnvironmentAdmissionInactivePreserved},
		{name: "materializing", snapshot: models.TaskEnvironmentAdmissionSnapshot{MaterializationSessionID: "session-materializing"}, want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "active turn", snapshot: snapshotWith(withActiveTurn(waiting)), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "starting session", snapshot: snapshotWith(admissionConsumer(models.TaskSessionStateStarting)), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "running session", snapshot: snapshotWith(admissionConsumer(models.TaskSessionStateRunning)), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "created session", snapshot: snapshotWith(admissionConsumer(models.TaskSessionStateCreated)), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "running executor", snapshot: snapshotWith(withExecutor(waiting, models.ExecutorRunningStatusRunning)), want: models.TaskEnvironmentAdmissionLiveBlocker},
		{name: "unknown executor status", snapshot: snapshotWith(withExecutor(waiting, "unknown")), want: models.TaskEnvironmentAdmissionLiveBlocker},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyAdmission(test.snapshot); got != test.want {
				t.Fatalf("ClassifyAdmission() = %q, want %q", got, test.want)
			}
		})
	}
}

func snapshotWith(consumer models.TaskEnvironmentAdmissionConsumer) models.TaskEnvironmentAdmissionSnapshot {
	return models.TaskEnvironmentAdmissionSnapshot{Consumers: []models.TaskEnvironmentAdmissionConsumer{consumer}}
}

func admissionConsumer(state models.TaskSessionState) models.TaskEnvironmentAdmissionConsumer {
	return models.TaskEnvironmentAdmissionConsumer{SessionID: "session-consumer", SessionState: state}
}

func withActiveTurn(consumer models.TaskEnvironmentAdmissionConsumer) models.TaskEnvironmentAdmissionConsumer {
	consumer.HasActiveTurn = true
	return consumer
}

func withExecutor(consumer models.TaskEnvironmentAdmissionConsumer, status string) models.TaskEnvironmentAdmissionConsumer {
	consumer.HasExecutor = true
	consumer.ExecutorStatus = status
	return consumer
}
