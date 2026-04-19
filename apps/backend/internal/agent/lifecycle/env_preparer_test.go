package lifecycle

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSerializePrepareResult(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		now := time.Now()
		twoSecsAgo := now.Add(-2 * time.Second)
		result := &EnvPrepareResult{
			Success: true,
			Steps: []PrepareStep{
				{Name: "Validate repository", Status: PrepareStepCompleted, Command: "git status", Output: "On branch main"},
				{Name: "Run setup script", Status: PrepareStepCompleted, Command: "npm install", Output: "added 100 packages", StartedAt: &twoSecsAgo, EndedAt: &now},
			},
			Duration: 3 * time.Second,
		}

		serialized := SerializePrepareResult(result)

		require.Equal(t, "completed", serialized["status"])
		require.Equal(t, int64(3000), serialized["duration_ms"])

		steps, ok := serialized["steps"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, steps, 2)

		require.Equal(t, "Validate repository", steps[0]["name"])
		require.Equal(t, string(PrepareStepCompleted), steps[0]["status"])
		require.Equal(t, "git status", steps[0]["command"])
		require.Equal(t, "On branch main", steps[0]["output"])

		require.Equal(t, "Run setup script", steps[1]["name"])
		require.NotEmpty(t, steps[1]["started_at"])
		require.NotEmpty(t, steps[1]["ended_at"])
	})

	t.Run("failure result", func(t *testing.T) {
		result := &EnvPrepareResult{
			Success:      false,
			ErrorMessage: "setup script failed",
			Steps: []PrepareStep{
				{Name: "Run setup script", Status: PrepareStepFailed, Command: "npm install", Output: "ERR! not found"},
			},
			Duration: 1 * time.Second,
		}

		serialized := SerializePrepareResult(result)

		require.Equal(t, "failed", serialized["status"])
		require.Equal(t, "setup script failed", serialized["error_message"])
	})

	t.Run("truncates long output at UTF-8 boundary", func(t *testing.T) {
		// Create output with a multi-byte emoji near the truncation boundary
		longOutput := strings.Repeat("a", MaxStepOutputBytes-2) + "\U0001F600" // 4-byte emoji

		result := &EnvPrepareResult{
			Success: true,
			Steps:   []PrepareStep{{Name: "test", Status: PrepareStepCompleted, Output: longOutput}},
		}

		serialized := SerializePrepareResult(result)
		steps := serialized["steps"].([]map[string]interface{})
		output := steps[0]["output"].(string)

		require.True(t, strings.HasSuffix(output, "\n... (truncated)"))
		// The output should be valid UTF-8 (no partial runes)
		require.True(t, strings.ToValidUTF8(output, "") == output, "output should be valid UTF-8")
	})

	t.Run("omits timestamps when nil", func(t *testing.T) {
		result := &EnvPrepareResult{
			Success: true,
			Steps:   []PrepareStep{{Name: "test", Status: PrepareStepCompleted}},
		}

		serialized := SerializePrepareResult(result)
		steps := serialized["steps"].([]map[string]interface{})
		_, hasStarted := steps[0]["started_at"]
		_, hasEnded := steps[0]["ended_at"]
		require.False(t, hasStarted, "should not include started_at when nil")
		require.False(t, hasEnded, "should not include ended_at when nil")
	})
}
