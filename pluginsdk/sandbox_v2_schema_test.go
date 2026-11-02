package pluginsdk

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestSandboxV2FieldsExtendExistingMessagesWithoutRenumbering(t *testing.T) {
	for _, tc := range []struct {
		message protoreflect.MessageDescriptor
		name    protoreflect.Name
	}{
		{(&SandboxPolicySnapshot{}).ProtoReflect().Descriptor(), "policy_v2"},
		{(&SandboxCompiledPolicy{}).ProtoReflect().Descriptor(), "effective_policy_v2"},
		{(&SandboxProviderDescription{}).ProtoReflect().Descriptor(), "capabilities_v2"},
		{(&NormalizeSandboxApprovalRequest{}).ProtoReflect().Descriptor(), "service_binding_v2"},
		{(&NormalizeSandboxApprovalResponse{}).ProtoReflect().Descriptor(), "approval_v2"},
		{(&NormalizeSandboxStatusResponse{}).ProtoReflect().Descriptor(), "observations_v2"},
		{(&NormalizeSandboxStatusRequest{}).ProtoReflect().Descriptor(), "source_relative_path_v2"},
		{(&ReadSandboxReportRequest{}).ProtoReflect().Descriptor(), "chunk_v2"},
		{(&ReadSandboxReportResponse{}).ProtoReflect().Descriptor(), "report_v2"},
		{(&ProposeSandboxRevisionResponse{}).ProtoReflect().Descriptor(), "additions_v2"},
	} {
		t.Run(string(tc.message.Name()), func(t *testing.T) {
			field := tc.message.Fields().ByName(tc.name)
			require.NotNil(t, field)
			require.Equal(t, protoreflect.FieldNumber(100), field.Number())
		})
	}
}

func TestAuditChunksBindEverySourceToTheCapturedSession(t *testing.T) {
	descriptor := (&SandboxAuditChunkV2{}).ProtoReflect().Descriptor()
	for number, name := range map[protoreflect.FieldNumber]protoreflect.Name{9: "source_schema", 10: "source_relative_path", 11: "provider_session_id", 12: "source_index", 13: "source_count"} {
		field := descriptor.Fields().ByNumber(number)
		require.NotNil(t, field, "missing source binding %s", name)
		require.Equal(t, name, field.Name())
	}
}

func TestTypedReportsAreNegotiatedSeparatelyFromAuditSignatures(t *testing.T) {
	descriptor := (&SandboxCapabilitiesV2{}).ProtoReflect().Descriptor()
	field := descriptor.Fields().ByName("report_schemas")
	require.NotNil(t, field)
	require.Equal(t, protoreflect.FieldNumber(7), field.Number())
}

func TestAuditEventsUseClosedReasonCodesInsteadOfRawErrors(t *testing.T) {
	field := (&SandboxAuditEventV2{}).ProtoReflect().Descriptor().Fields().ByName("reason_code")
	require.NotNil(t, field)
	require.Equal(t, protoreflect.FieldNumber(11), field.Number())
}
