package executor

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestSandboxRestartReviewSurvivesResumeRequest(t *testing.T) {
	var options ResumeOptions
	require.NoError(t, json.Unmarshal([]byte(`{"SandboxRestartReview":{"attempt_id":"attempt","selection_digest":"digest"}}`), &options))
	request, _ := newResumeLaunchRequest(&v1.Task{ID: "task"}, &models.TaskSession{ID: "session"}, true, options)
	review := reflect.ValueOf(request).Elem().FieldByName("SandboxRestartReview")
	require.True(t, review.IsValid(), "resume must carry the explicit review to launch admission")
	require.False(t, review.IsNil())
	raw, err := json.Marshal(review.Interface())
	require.NoError(t, err)
	require.JSONEq(t, `{"attempt_id":"attempt","selection_digest":"digest"}`, string(raw))
}
