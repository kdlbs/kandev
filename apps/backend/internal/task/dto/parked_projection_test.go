package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeParkedProvider struct {
	parked   bool
	revision uint64
	epoch    uint64
}

func (p fakeParkedProvider) ParkedSnapshot(string) (bool, uint64) {
	return p.parked, p.revision
}

func (p fakeParkedProvider) ParkedEpoch() uint64 {
	return p.epoch
}

type fakeTaskParkedProvider struct {
	parked   bool
	revision uint64
	epoch    uint64
}

func (p fakeTaskParkedProvider) TaskParkedSnapshot(string) (bool, uint64) {
	return p.parked, p.revision
}

func (p fakeTaskParkedProvider) ParkedEpoch() uint64 {
	return p.epoch
}

// AC-50-adjacent: the default wire shape (no enrichment call at all) reads
// false/0/0 — the D9 default — so a client applying the JSON body verbatim
// gets the conservative reading rather than a missing field.
func TestParkedProjectionSerializesExplicitDefaults(t *testing.T) {
	for name, value := range map[string]any{
		"session-full":    TaskSessionDTO{},
		"session-summary": TaskSessionSummaryDTO{},
		"task":            TaskDTO{},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)

			var body map[string]any
			require.NoError(t, json.Unmarshal(encoded, &body))
			require.Contains(t, body, "parked_on_background_work")
			require.Equal(t, false, body["parked_on_background_work"])
			require.Contains(t, body, "parked_epoch")
			require.Equal(t, float64(0), body["parked_epoch"])
		})
	}

	encoded, err := json.Marshal(TaskSessionDTO{})
	require.NoError(t, err)
	var sessionBody map[string]any
	require.NoError(t, json.Unmarshal(encoded, &sessionBody))
	require.Contains(t, sessionBody, "revision")
	require.Equal(t, float64(0), sessionBody["revision"])

	encoded, err = json.Marshal(TaskDTO{})
	require.NoError(t, err)
	var taskBody map[string]any
	require.NoError(t, json.Unmarshal(encoded, &taskBody))
	require.Contains(t, taskBody, "parked_revision")
	require.Equal(t, float64(0), taskBody["parked_revision"])
}

func TestEnrichParkedProjectionWritesProviderValue(t *testing.T) {
	provider := fakeParkedProvider{parked: true, revision: 7, epoch: 42}

	full := &TaskSessionDTO{ID: "s1"}
	EnrichParkedProjection(full, provider)
	require.True(t, full.ParkedOnBackgroundWork)
	require.Equal(t, uint64(7), full.Revision)
	require.Equal(t, uint64(42), full.ParkedEpoch)

	summary := &TaskSessionSummaryDTO{ID: "s1"}
	EnrichParkedProjectionSummary(summary, provider)
	require.True(t, summary.ParkedOnBackgroundWork)
	require.Equal(t, uint64(7), summary.Revision)
	require.Equal(t, uint64(42), summary.ParkedEpoch)
}

// A settled provider must clear stale client state (true -> false), the same
// contract EnrichCancellationPending has for its boolean.
func TestEnrichParkedProjectionClearsStaleState(t *testing.T) {
	provider := fakeParkedProvider{parked: false, revision: 3, epoch: 42}

	full := &TaskSessionDTO{ID: "s1", ParkedOnBackgroundWork: true, Revision: 1}
	EnrichParkedProjection(full, provider)
	require.False(t, full.ParkedOnBackgroundWork)
	require.Equal(t, uint64(3), full.Revision)
}

func TestEnrichParkedProjectionNilSafety(t *testing.T) {
	// Nil DTO must not panic.
	EnrichParkedProjection(nil, fakeParkedProvider{parked: true})
	EnrichParkedProjectionSummary(nil, fakeParkedProvider{parked: true})
	EnrichTaskParkedProjection(nil, fakeTaskParkedProvider{parked: true})

	// Nil provider (unwired — matches the pre-feature/test-double-less default)
	// must leave the DTO's zero value untouched rather than panicking.
	full := &TaskSessionDTO{ID: "s1"}
	EnrichParkedProjection(full, nil)
	require.False(t, full.ParkedOnBackgroundWork)

	task := &TaskDTO{ID: "t1"}
	EnrichTaskParkedProjection(task, nil)
	require.False(t, task.ParkedOnBackgroundWork)
}

// AC-22/AC-50: EnrichTaskParkedProjection stamps the task's own OR-aggregate
// and revision — it must not touch ForegroundActivity or any other field.
func TestEnrichTaskParkedProjectionWritesProviderValueOnly(t *testing.T) {
	provider := fakeTaskParkedProvider{parked: true, revision: 3, epoch: 99}

	task := &TaskDTO{ID: "t1", ForegroundActivity: "generating"}
	EnrichTaskParkedProjection(task, provider)

	require.True(t, task.ParkedOnBackgroundWork)
	require.Equal(t, uint64(3), task.ParkedRevision)
	require.Equal(t, uint64(99), task.ParkedEpoch)
	require.EqualValues(t, "generating", task.ForegroundActivity)
}
