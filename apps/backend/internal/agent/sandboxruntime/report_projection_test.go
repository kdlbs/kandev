package sandboxruntime

import (
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestReportProjectionNeverCarriesCommandArgumentsOrQueryValues(t *testing.T) {
	kind, operation := projectReportOperation(&pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Command{Command: &pluginsdk.SandboxCommandInvocationV2{CommandId: "deploy", Caller: "agent", ExecutablePath: "/private/tool", ExecutableSha256: "private", Argv: []string{"secret"}, WorkDirReference: "work", ResourcePolicyDigest: "private"}}})
	require.Equal(t, "command", kind)
	require.Equal(t, policy.ReportOperation{Command: &policy.ReportCommand{ID: "deploy", Caller: "agent"}}, operation)
	kind, operation = projectReportOperation(&pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_HttpEndpoint{HttpEndpoint: &pluginsdk.SandboxHTTPEndpointV2{RouteId: "route", Origin: "https://example.test", Method: "GET", Path: "/public", Query: "secret=private"}}})
	require.Equal(t, "http_endpoint", kind)
	require.Equal(t, policy.ReportOperation{Endpoint: &policy.ReportEndpoint{RouteID: "route", Origin: "https://example.test", Method: "GET", Path: "/public"}}, operation)
}

func TestReportProjectionBridgesOnlyExactDeniedFilesystemRecords(t *testing.T) {
	for _, decision := range []string{"denied", "approval_denied", "allowed", "approval_requested", "approval_granted"} {
		t.Run(decision, func(t *testing.T) {
			data := policy.ReportData{V2: &policy.TypedReport{}}
			event := &pluginsdk.SandboxAuditEventV2{RecordId: "request", SourceId: "source", Decision: decision, ExactTarget: true, Operation: &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Filesystem{Filesystem: &pluginsdk.SandboxFilesystemOperationV2{Path: "/work/file", Access: "read"}}}}
			appendTypedEvent(&data, event)
			if decision == "denied" || decision == "approval_denied" {
				require.Len(t, data.Records, 1)
				require.Equal(t, data.V2.Records[0].LegacyRecordID, data.Records[0].ID)
			} else {
				require.Empty(t, data.Records)
				require.Equal(t, "not_denied", data.V2.Records[0].LearningBlocker)
			}
			event.ExactTarget = false
			data = policy.ReportData{V2: &policy.TypedReport{}}
			appendTypedEvent(&data, event)
			require.Empty(t, data.Records)
			require.Equal(t, "incomplete_identity", data.V2.Records[0].LearningBlocker)
		})
	}
}

func TestTypedReadWriteDenialKeepsExistingFilesystemLearning(t *testing.T) {
	data := policy.ReportData{V2: &policy.TypedReport{}}
	appendTypedEvent(&data, &pluginsdk.SandboxAuditEventV2{RecordId: "request", SourceId: "source", Decision: "denied", ExactTarget: true, Operation: &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Filesystem{Filesystem: &pluginsdk.SandboxFilesystemOperationV2{Path: "/work/file", Access: "read_write"}}}})
	require.Len(t, data.Records, 1)
	require.Equal(t, "write", data.Records[0].Operation, "the existing writable grant includes read and write")
	require.Equal(t, data.Records[0].ID, data.V2.Records[0].LegacyRecordID)
}
