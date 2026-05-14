package docker

import (
	"context"
	"testing"
	"time"
)

func TestContainerResponsesIncludeLabels(t *testing.T) {
	containers := []ContainerInfo{{
		ID:        "container-1",
		Name:      "kandev-agent-1",
		Image:     "kandev/agent:test",
		State:     "running",
		Status:    "Up 2 seconds",
		StartedAt: time.Unix(100, 0),
		Labels: map[string]string{
			"kandev.executor_profile_id": "profile-1",
			"kandev.task_id":             "task-1",
		},
	}}

	got := newContainerResponses(containers)

	if got[0].Labels["kandev.executor_profile_id"] != "profile-1" {
		t.Fatalf("labels = %#v, want executor profile label", got[0].Labels)
	}
	if got[0].Labels["kandev.task_id"] != "task-1" {
		t.Fatalf("labels = %#v, want task label", got[0].Labels)
	}
}

func TestContainerResponsesEnrichTaskTitleLabel(t *testing.T) {
	containers := []ContainerInfo{{
		ID:     "container-1",
		Name:   "kandev-agent-1",
		Image:  "kandev/agent:test",
		State:  "running",
		Status: "Up 2 seconds",
		Labels: map[string]string{
			"kandev.task_id": "task-1",
		},
	}}

	got := newContainerResponsesWithTaskTitles(context.Background(), containers, func(_ context.Context, taskID string) (string, bool) {
		return "Readable Task Title", taskID == "task-1"
	})

	if got[0].Labels["kandev.task_title"] != "Readable Task Title" {
		t.Fatalf("labels = %#v, want task title label", got[0].Labels)
	}
	if _, exists := containers[0].Labels["kandev.task_title"]; exists {
		t.Fatalf("source labels mutated: %#v", containers[0].Labels)
	}
}
