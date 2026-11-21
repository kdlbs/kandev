package pluginsdk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageCanonicalMetricApplicabilityOverWire(t *testing.T) {
	var rows []SessionUsageMeasurement
	require.NoError(t, json.Unmarshal([]byte(`[
 {"ID":"tokens","TotalTokens":100,"CostMetricAbsent":true},
 {"ID":"cost","CostSubcents":20,"TokenMetricAbsent":true},
 {"ID":"unknown"},
 {"ID":"zero","TotalTokens":0,"CostSubcents":0}
]`), &rows))
	host := dialHostOverBufconn(t, &dataRecordingHost{canonicalUsage: rows})
	got, _, err := host.Usage().ListCanonical(t.Context(), SessionUsageFilter{}, Page{})
	require.NoError(t, err)
	require.Len(t, got, 4)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	var output []map[string]any
	require.NoError(t, json.Unmarshal(encoded, &output))
	require.Equal(t, true, output[0]["CostMetricAbsent"])
	require.Equal(t, true, output[1]["TokenMetricAbsent"])
	require.Equal(t, false, output[2]["TokenMetricAbsent"])
	require.Equal(t, false, output[2]["CostMetricAbsent"])
	require.Nil(t, got[2].TotalTokens)
	require.Nil(t, got[2].CostSubcents)
	require.NotNil(t, got[3].TotalTokens)
	require.NotNil(t, got[3].CostSubcents)
	require.Zero(t, *got[3].TotalTokens)
	require.Zero(t, *got[3].CostSubcents)
}
