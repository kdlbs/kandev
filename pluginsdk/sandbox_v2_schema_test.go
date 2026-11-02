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
