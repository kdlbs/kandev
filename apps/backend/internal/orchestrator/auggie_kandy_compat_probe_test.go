package orchestrator

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Kandy Token Grotto's normalizeTokenUsage (plugin) requires:
// - usage map with integer-ish float64 fields (json numbers)
// - timestamp parseable as RFC3339Nano (which accepts RFC3339)
// - estimated either absent or bool
// - total_tokens > 0 OR input+output > 0
func TestAuggieDiskPayload_KandyNormalizeCompatible(t *testing.T) {
	usage := &streams.PromptUsage{
		InputTokens:       16,
		OutputTokens:      30,
		CachedReadTokens:  130,
		CachedWriteTokens: 52,
		TotalTokens:       46,
		Estimated:         false,
	}
	payload := lifecycle.SessionPromptUsageEventPayload{
		TaskID:    "t-auggie",
		SessionID: "s-auggie",
		AgentID:   "exec-auggie",
		AgentType: "auggie",
		Model:     "claude-opus-4-8",
		Usage:     usage,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	// timestamp: Kandy uses time.Parse(RFC3339Nano, ...)
	ts, ok := asMap["timestamp"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, ts)
	require.NoError(t, err, "Kandy rejects timestamps that fail RFC3339Nano parse")

	usageMap, ok := asMap["usage"].(map[string]any)
	require.True(t, ok)

	for _, name := range []string{
		"input_tokens", "output_tokens", "cached_read_tokens",
		"cached_write_tokens", "total_tokens",
	} {
		v, present := usageMap[name]
		require.Truef(t, present, "%s missing from JSON (omitempty?)", name)
		n, ok := v.(float64)
		require.Truef(t, ok, "%s type %T want float64 after json roundtrip", name, v)
		require.False(t, math.IsNaN(n) || math.IsInf(n, 0))
		require.GreaterOrEqual(t, n, 0.0)
		require.Less(t, n, float64(1<<53))
		require.Equal(t, math.Trunc(n), n)
	}

	// thought_tokens is omitempty; Kandy treats missing as Present=false, valid.
	// estimated is omitempty when false — Kandy accepts absence.
	if rawEst, present := usageMap["estimated"]; present {
		_, ok := rawEst.(bool)
		require.True(t, ok, "estimated present but not bool")
	}

	require.Equal(t, "auggie", asMap["agent_type"])
	require.Equal(t, "claude-opus-4-8", asMap["model"])
}
