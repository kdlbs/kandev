package codexappserver

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementRequiresSettledNativeHistoryAndFreshCatalog(t *testing.T) {
	for _, scenario := range []string{"settled", "running", "empty", "disconnected", "paginated", "wrong-thread", "unknown-child", "settled-child"} {
		t.Run(scenario, func(t *testing.T) {
			server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
				id := req["id"]
				switch readString(req, "method") {
				case "initialize":
					return write(resultFrame(id, map[string]any{"userAgent": "codex/0.166.0"}))
				case "initialized":
					return nil
				case "model/list":
					return write(resultFrame(id, map[string]any{"data": []any{}}))
				case "thread/read":
					status := "completed"
					if scenario == "running" {
						status = "inProgress"
					}
					turns := []any{map[string]any{"id": "turn", "status": status, "startedAt": int64(1700000000)}}
					if scenario == "empty" {
						turns = nil
					}
					thread := "native"
					if scenario == "wrong-thread" {
						thread = "other"
					}
					return write(resultFrame(id, map[string]any{"thread": map[string]any{"id": thread, "turns": turns}}))
				case "thread/backgroundTerminals/list":
					return write(resultFrame(id, map[string]any{"data": []any{}}))
				case "mcpServerStatus/list":
					state := "connected"
					if scenario == "disconnected" {
						state = "failed"
					}
					result := map[string]any{"data": []any{map[string]any{"name": "kandev", "runtimeStatus": state, "tools": map[string]any{"fresh_tool": map[string]any{}}}}}
					if scenario == "paginated" {
						result["nextCursor"] = "next"
					}
					return write(resultFrame(id, result))
				default:
					t.Errorf("unexpected method: %s", readString(req, "method"))
					return write(errorFrame(id, -32601, "unsupported"))
				}
			})
			defer server.close()
			a := NewAdapter(&shared.Config{}, logger.Default())
			defer func() { _ = a.Close() }()
			require.NoError(t, a.Connect(server.clientWriter, server.clientReader))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, a.Initialize(ctx))
			a.mu.Lock()
			a.threadID = "native"
			if scenario == "unknown-child" || scenario == "settled-child" {
				a.children["child"] = childBinding{activeTurnID: "old-turn"}
				if scenario == "settled-child" {
					a.childStatuses["child"] = "completed"
				} else {
					a.children["child"] = childBinding{}
				}
			}
			a.mu.Unlock()
			replacement, ok := any(a).(interface {
				ProviderMCPReplacementReady(context.Context) error
				ProviderMCPCatalog(context.Context, string) (map[string][]string, error)
			})
			require.True(t, ok, "native transport must expose verified replacement support")
			err := replacement.ProviderMCPReplacementReady(ctx)
			if scenario == "running" || scenario == "empty" || scenario == "wrong-thread" || scenario == "unknown-child" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			recorder, ok := any(a).(interface {
				RecordedTurnStart(string) (time.Time, bool)
			})
			require.True(t, ok, "replacement needs a native turn timestamp for workload evidence")
			started, ok := recorder.RecordedTurnStart("native")
			require.True(t, ok)
			require.Equal(t, time.Unix(1700000000, 0), started)
			catalog, err := replacement.ProviderMCPCatalog(ctx, "native")
			if scenario == "disconnected" || scenario == "paginated" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string][]string{"kandev": {"fresh_tool"}}, catalog)
		})
	}
}

func TestProviderReplacementSupportRequiresProvenNativeVersion(t *testing.T) {
	a := NewAdapter(&shared.Config{}, logger.Default())
	support, ok := any(a).(interface{ ProviderMCPReplacementSupported() bool })
	require.True(t, ok, "support must be separate from current activity")
	for _, version := range []string{"codex/0.162.1", "codex/0.166.0", "codex/0.167.0", "codex/0.169.0", "codex/0.169.1", ""} {
		a.mu.Lock()
		a.providerVersion = version
		a.mu.Unlock()
		require.Equal(t, (version == "codex/0.166.0" || version == "codex/0.169.0") && runtime.GOOS == "linux", support.ProviderMCPReplacementSupported())
	}
}
