package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveSubagentCountSerializesExplicitZero(t *testing.T) {
	tests := map[string]any{
		"task":            TaskDTO{},
		"session":         TaskSessionDTO{},
		"session summary": TaskSessionSummaryDTO{},
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)

			var body map[string]any
			require.NoError(t, json.Unmarshal(encoded, &body))
			assert.Equal(t, float64(0), body["active_subagent_count"])
		})
	}
}
