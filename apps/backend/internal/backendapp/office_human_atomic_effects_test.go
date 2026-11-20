package backendapp

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

// Compare persisted effects as well as the task DTO: a refusal must not leave
// a marker, decision, history, activity or queued run behind.
func officeAtomicDurableEffects(t *testing.T, f officeAtomicFixture) map[string][]map[string]any {
	t.Helper()
	result := make(map[string][]map[string]any)
	for _, table := range []string{"tasks", "task_sessions", "task_step_transitions",
		"task_completion_gate_history", "task_completion_gate_operations", "workflow_step_entries",
		"workflow_step_entry_markers", "workflow_step_decisions", "workflow_step_participants",
		"office_activity_log", "runs", "run_events"} {
		rows, err := f.h.db.Queryx(`SELECT * FROM ` + table + ` ORDER BY 1,2`)
		require.NoError(t, err)
		var records []map[string]any
		for rows.Next() {
			record := make(map[string]any)
			require.NoError(t, rows.MapScan(record))
			for key, value := range record {
				if data, ok := value.([]byte); ok {
					record[key] = string(data)
				}
			}
			records = append(records, record)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		result[table] = records
	}
	return result
}

func officeAtomicPublishedEvents(t *testing.T, f officeAtomicFixture) func() []string {
	t.Helper()
	var mu sync.Mutex
	var subjects []string
	subscription, err := f.h.eventBus.Subscribe(">", func(_ context.Context, event *bus.Event) error {
		mu.Lock()
		subjects = append(subjects, event.Subject)
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, subscription.Unsubscribe()) })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), subjects...)
	}
}
