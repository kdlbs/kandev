package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// A cold native resume after continue_from_history omits a historical
// submission the peer no longer retains. That omission must not reject the
// continued notice; a retained but contradictory peer record still must.
func TestNativeResumeProofAdmitsHistoricalNotice(t *testing.T) {
	recovery := models.AgentDeliveryRecovery{SubmissionID: "predecessor", StreamID: "stream-1", HarnessGeneration: 1}
	current := models.NativeResumeSubmissionEvidence{ID: "current", HarnessGeneration: 2, StreamID: "stream-2"}

	omitted := &models.NativeResumeDeliveryEvidence{Submissions: []models.NativeResumeSubmissionEvidence{current}}
	require.True(t, nativeResumeProofAdmitsHistoricalNotice(omitted, recovery), "a record the peer no longer retains falls through to the SQL ancestry checks")

	retained := &models.NativeResumeDeliveryEvidence{Submissions: []models.NativeResumeSubmissionEvidence{
		current, {ID: "predecessor", HarnessGeneration: 1, StreamID: "stream-1"},
	}}
	require.True(t, nativeResumeProofAdmitsHistoricalNotice(retained, recovery))

	contradictory := &models.NativeResumeDeliveryEvidence{Submissions: []models.NativeResumeSubmissionEvidence{
		current, {ID: "predecessor", HarnessGeneration: 1, StreamID: "stream-other"},
	}}
	require.False(t, nativeResumeProofAdmitsHistoricalNotice(contradictory, recovery), "retained peer evidence must still match the notice")
}
