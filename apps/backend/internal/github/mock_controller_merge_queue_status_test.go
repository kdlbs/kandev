package github

import (
	"testing"
	"time"
)

func TestBuildTaskPRFromRequestCopiesMergeQueueStatus(t *testing.T) {
	position := 9
	estimate := 360
	req := &associateTaskPRRequest{
		TaskID: "task-queue-fixture", PRNumber: 42, State: "open",
		MergeQueueState: "queued", MergeQueuePosition: &position,
		MergeQueueEstimatedTimeToMergeSeconds: &estimate,
	}
	tp := buildTaskPRFromRequest(req, time.Now().UTC())
	assertTaskPRMergeQueueFields(t, "mock fixture", tp, "queued", &position, &estimate)
}
