package sandbox

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObservationRetainsIndependentFacts(t *testing.T) {
	raw := `{"attempt_id":"attempt","generation":1,"attempt_version":1,"status":"unknown","provider_health":"unavailable","reason":"provider_unavailable","checked_at":"2026-11-02T12:00:00Z","facts":{"schema_version":1,"process":{"state":"running","checked_at":"2026-11-02T12:00:00Z","valid_until":"2026-11-02T12:00:30Z"},"launch_checks":{"state":"unavailable"},"runtime":{"state":"missing"},"audit":{"integrity":"unchecked","finalization":"unknown","signer":"unchecked"}}}`
	var value Observation
	require.NoError(t, json.Unmarshal([]byte(raw), &value))
	require.True(t, value.ValidFields())
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, raw, string(encoded), "process running must coexist with unavailable provider and unknown confinement")
	for _, change := range []struct{ old, new string }{
		{`"schema_version":1`, `"schema_version":2`},
		{`"state":"running"`, `"state":"protected"`},
		{`12:00:30Z`, `12:00:31Z`},
		{`"integrity":"unchecked"`, `"integrity":"trusted"`},
	} {
		var invalid Observation
		require.NoError(t, json.Unmarshal([]byte(strings.Replace(raw, change.old, change.new, 1)), &invalid))
		require.False(t, invalid.ValidFields(), change.new)
	}
}

func TestLegacyObservationRemainsValidWithoutFacts(t *testing.T) {
	value := Observation{AttemptID: "a", Generation: 1, AttemptVersion: 1, Status: StatusUnknown, ProviderHealth: HealthUnknown, CheckedAt: time.Now().UTC()}
	require.True(t, value.ValidFields())
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"facts"`)
}

func TestLaunchReceiptRejectsIncompleteChecksAndUnboundDigests(t *testing.T) {
	now := time.Now().UTC()
	receipt := LaunchCheckReceipt{CheckedAt: now, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), RuntimeDigest: strings.Repeat("c", 64), Checks: []string{"policy_ceiling", "runtime_checksum", "private_artifacts", "process_containment"}}
	require.True(t, receipt.Valid(now))
	for _, mutate := range []func(*LaunchCheckReceipt){
		func(r *LaunchCheckReceipt) { r.CheckedAt = now.Add(time.Second) },
		func(r *LaunchCheckReceipt) { r.RuntimeDigest = "unverified" },
		func(r *LaunchCheckReceipt) { r.Checks = r.Checks[:3] },
		func(r *LaunchCheckReceipt) { r.Checks = []string{"kernel_enforcement"} },
	} {
		invalid := receipt
		mutate(&invalid)
		require.False(t, invalid.Valid(now))
	}
}
