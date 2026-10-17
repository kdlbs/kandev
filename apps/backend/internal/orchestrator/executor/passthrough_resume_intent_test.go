package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestPassthroughResumeIntentSurvivesRuntimeCleanup(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, continuation := range []bool{false, true} {
			req, _ := newResumeLaunchRequest(&v1.Task{ID: "task"},
				&models.TaskSession{ID: "session", IsPassthrough: passthrough}, true,
				ResumeOptions{ForceContextContinuation: continuation})
			require.Equal(t, passthrough && !continuation, req.ResumePassthrough)
			require.Empty(t, req.PreviousExecutionID, "conversation resume must not invent a runtime owner")
		}
	}
}
