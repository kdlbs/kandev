package sandboxruntime

import (
	"testing"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/stretchr/testify/require"
)

func TestFailedFinalReportKeepsReadAgeAndMarksAnInterruptedCollection(t *testing.T) {
	captured := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	previous := &policy.Report{CapturedAt: captured, Data: policy.ReportData{Status: reportStatusAvailable, Integrity: "partial", V2: &policy.TypedReport{Integrity: "valid", Finalization: "live"}}}
	data := settleReport(unavailableReport(previous), true)
	require.True(t, data.Finalized)
	require.Equal(t, "interrupted", data.V2.Finalization)
	require.Equal(t, "valid", data.V2.Integrity)
	require.Equal(t, &captured, data.LastSuccessfulReadAt)
	require.Equal(t, "live", previous.Data.V2.Finalization, "the prior immutable report remains unchanged")
}
