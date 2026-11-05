package sandbox

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTypedReportRetainsIndependentAxesAndPartialTargets(t *testing.T) {
	raw := typedReportFixtureJSON(t)
	var data ReportData
	require.NoError(t, json.Unmarshal(raw, &data))
	require.True(t, validReportData(data, time.Now().UTC()))
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(encoded), "typed report evidence must survive persistence without changing its meaning")
}

func TestTypedReportRejectsInventedExactIdentitiesAndRawArguments(t *testing.T) {
	raw := typedReportFixtureJSON(t)
	for _, change := range []struct{ old, new string }{
		{`"schema_version":2`, `"schema_version":3`},
		{`"exact_target":false`, `"exact_target":true`},
		{`"kind":"network"`, `"kind":"filesystem"`},
		{`"integrity":"valid"`, `"integrity":"complete"`},
		{`"finalization":"live"`, `"finalization":"protected"`},
	} {
		var data ReportData
		require.NoError(t, json.Unmarshal([]byte(strings.Replace(string(raw), change.old, change.new, 1)), &data))
		require.False(t, validReportData(data, time.Now().UTC()), change.new)
	}
}

func typedReportFixtureJSON(t *testing.T) []byte {
	t.Helper()
	data := reportFixture()
	data.Sources[0].ID = strings.Repeat("a", 64)
	data.Sources[0].Schema = "fixture.audit.v2"
	data.Records[0].SourceID = data.Sources[0].ID
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(encoded, &envelope))
	envelope["v2"] = map[string]any{
		"schema_version": 2, "artifact_set_digest": strings.Repeat("b", 64), "integrity": "valid", "finalization": "live",
		"coverage": []map[string]string{{"kind": "filesystem", "coverage": "partial"}, {"kind": "network", "coverage": "partial"}, {"kind": "command", "coverage": "unsupported"}, {"kind": "http_endpoint", "coverage": "unsupported"}},
		"sources":  []map[string]any{{"id": strings.Repeat("a", 64), "schema": "fixture.audit.v2", "digest": strings.Repeat("a", 64), "identity": strings.Repeat("c", 64), "bytes": 6}},
		"records":  []map[string]any{{"id": strings.Repeat("d", 64), "source_id": strings.Repeat("a", 64), "provider_record_id": "event-1", "sequence": 1, "kind": "network", "decision": "denied", "reason_code": "permission_denied", "occurred_at": data.Records[0].OccurredAt.Format(time.RFC3339Nano), "exact_target": false, "learning_blocker": "incomplete_identity", "operation": map[string]any{"tcp": map[string]any{"host": "", "port": 8443, "transport": "tcp"}}}},
	}
	raw, err := json.Marshal(envelope)
	require.NoError(t, err)
	return raw
}

func TestTypedReportOperationRejectsRawArguments(t *testing.T) {
	raw := typedReportFixtureJSON(t)
	raw = []byte(strings.Replace(string(raw), `"transport":"tcp"`, `"transport":"tcp","argv":["synthetic-private"]`, 1))
	var data ReportData
	require.Error(t, json.Unmarshal(raw, &data), "public report operations have no raw argv field")
}
