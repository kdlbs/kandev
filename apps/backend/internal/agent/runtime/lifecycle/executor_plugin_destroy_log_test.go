package lifecycle

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func observedDestroyLog(t *testing.T, runtime *PluginRemoteExecutor) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	wrapped, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	runtime.logger = wrapped
	return logs
}

func destroyLogFields(t *testing.T, logs *observer.ObservedLogs) map[string]interface{} {
	t.Helper()
	entries := logs.FilterMessage("plugin executor destroy").All()
	require.Len(t, entries, 1, "exactly one structured line per destroy")
	return entries[0].ContextMap()
}

// Each way a destroy ends unconfirmed names itself in the log, with the evidence
// the provider received and answered, and never a secret.
func TestDestroyLogsWhichUnconfirmedConditionFired(t *testing.T) {
	cases := map[string]struct {
		arrange func(*pluginExecutorOperationsFake)
		want    string
	}{
		"call error": {func(o *pluginExecutorOperationsFake) { o.destroyErr = errors.New("transport down") }, "call_error"},
		"error response": {func(o *pluginExecutorOperationsFake) {
			o.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{Error: &pluginsdk.ExecutorProviderError{Code: "retained", MessageId: "msg-host-unsupported"}}
		}, "error_response"},
		"not confirmed absent": {func(o *pluginExecutorOperationsFake) {
			o.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: false}
		}, "not_confirmed_absent"},
		"claim checkpoint missing": {func(o *pluginExecutorOperationsFake) {}, "claim_checkpoint_missing"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			claimFor := checkpointedUnder(1)
			if tc.want == "claim_checkpoint_missing" {
				claimFor = nil
			}
			runtime, operations, _ := claimSettlementRuntime(t, "ready", claimFor, workspaceResource("", ""))
			tc.arrange(operations)
			logs := observedDestroyLog(t, runtime)

			require.Error(t, destroyClaimEnvironment(runtime))
			fields := destroyLogFields(t, logs)
			require.Equal(t, tc.want, fields["unconfirmed_condition"])
			require.Equal(t, "task_environment_reset", fields["cleanup_reason"])
			require.NotEmpty(t, fields["cleanup_claim"])
			require.Contains(t, fields, "drain_outcome")
			require.Contains(t, fields, "drain_receipt_attached")
			require.Contains(t, fields, "confirmed_absent")
			if tc.want == "error_response" {
				require.Equal(t, "retained", fields["error_code"])
				require.Equal(t, "msg-host-unsupported", fields["error_message_id"])
			}
		})
	}
}

func TestDestroyLogsAConfirmedDestroy(t *testing.T) {
	runtime, _, _ := claimSettlementRuntime(t, "ready", checkpointedUnder(1), workspaceResource("", ""))
	logs := observedDestroyLog(t, runtime)
	require.NoError(t, destroyClaimEnvironment(runtime))
	fields := destroyLogFields(t, logs)
	require.Equal(t, "", fields["unconfirmed_condition"])
	require.Equal(t, true, fields["confirmed_absent"])
}
