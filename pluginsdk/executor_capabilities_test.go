package pluginsdk

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestValidateProfileCapabilityFieldsRoundTrip(t *testing.T) {
	request := &ValidateExecutorProfileRequest{
		Profile: &ExecutorProfileSnapshot{ProfileId: "p"},
		HostCapabilities: []*ExecutorHostCapability{
			{Name: "workspace_descriptor", MinVersion: 1, MaxVersion: 1},
		},
	}
	encoded, err := proto.Marshal(request)
	require.NoError(t, err)
	var decoded ValidateExecutorProfileRequest
	require.NoError(t, proto.Unmarshal(encoded, &decoded))
	require.Equal(t, "workspace_descriptor", decoded.GetHostCapabilities()[0].GetName())

	response := &ValidateExecutorProfileResponse{
		ImplementedCapabilities: []*ExecutorCapabilityVersion{{Name: "workspace_descriptor", Version: 1}},
	}
	encoded, err = proto.Marshal(response)
	require.NoError(t, err)
	var echoed ValidateExecutorProfileResponse
	require.NoError(t, proto.Unmarshal(encoded, &echoed))
	require.Equal(t, uint32(1), echoed.GetImplementedCapabilities()[0].GetVersion())
}

func TestLegacyValidationMessagesCarryNoCapabilityFields(t *testing.T) {
	legacy := &ValidateExecutorProfileResponse{Capabilities: &ExecutorProviderCapabilities{Files: true}}
	encoded, err := proto.Marshal(legacy)
	require.NoError(t, err)
	var decoded ValidateExecutorProfileResponse
	require.NoError(t, proto.Unmarshal(encoded, &decoded))
	require.Empty(t, decoded.GetImplementedCapabilities())
}
