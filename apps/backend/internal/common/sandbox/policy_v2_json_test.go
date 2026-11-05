package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyV2DecodeRefusesUnsupportedPermissionFields(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":2,"static_grants":{"commands":[{"command_id":"build"}]},"approval_envelope":{}}`,
		`{"schema_version":2,"static_grants":{},"approval_envelope":{"http_endpoints":[{"route_id":"api"}]}}`,
		`{"schema_version":2,"static_grants":{},"approval_envelope":{},"resource_policies":[]}`,
		`{"schema_version":2,"static_grants":{"tcp":[{"host":"api.example.test","port":443,"transport":"tcp","path":"/private"}]},"approval_envelope":{}}`,
	} {
		var policy PolicyV2
		require.Error(t, json.Unmarshal([]byte(raw), &policy), "a reader must not erase unsupported restrictions")
	}
	var supported PolicyV2
	err := json.Unmarshal([]byte(`{"schema_version":2,"static_grants":{"filesystem":[],"tcp":[{"host":"api.example.test","port":443,"transport":"tcp"}],"kandev_mcp":false},"approval_envelope":{"filesystem":[],"tcp":[],"kandev_mcp":false}}`), &supported)
	require.NoError(t, err)
	require.Equal(t, uint16(443), supported.StaticGrants.TCP[0].Port)
}
