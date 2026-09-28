package handlers

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/stretchr/testify/assert"
)

// Container network placement decides what a task container can reach: an
// internal network denies egress, a macvlan gives it an address on the
// operator's physical LAN. An agent that could set these keys on a profile it
// creates would choose its own containment, so they belong to the operator
// path alongside allow_user_namespaces.
//
// A prepare script cannot cross this boundary, so the "operator already
// exposes prepare_script, which is strictly more powerful" argument that
// keeps other keys off this list does not apply.
func TestRejectOperatorConfigKeys_RejectsContainerNetworkKeys(t *testing.T) {
	for _, key := range []string{
		lifecycle.MetadataKeyDockerNetwork,
		lifecycle.MetadataKeyDockerNetworkGwPriority,
		lifecycle.MetadataKeyDockerAdditionalNetworks,
	} {
		t.Run(key, func(t *testing.T) {
			err := rejectOperatorConfigKeys(map[string]string{key: "lab-bridge"})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), key)
		})
	}
}

// An agent updating a profile must not be able to drop the operator's network
// placement by omitting the keys.
func TestPreserveOperatorConfigKeys_KeepsContainerNetworks(t *testing.T) {
	current := map[string]string{
		lifecycle.MetadataKeyDockerNetwork:            "internal-only",
		lifecycle.MetadataKeyDockerAdditionalNetworks: `[{"name":"metrics"}]`,
	}

	preserved := preserveOperatorConfigKeys(map[string]string{"image_tag": "x:1"}, current)

	assert.Equal(t, "internal-only", preserved[lifecycle.MetadataKeyDockerNetwork])
	assert.Equal(t, `[{"name":"metrics"}]`, preserved[lifecycle.MetadataKeyDockerAdditionalNetworks])
}

// The local key constants exist because this tier must not import
// internal/agent/runtime/lifecycle. Tests may, so this is where the two
// spellings are held together: a rename on either side fails here rather than
// silently unguarding the agent-facing profile tools.
func TestContainerNetworkConfigKeysMatchRuntime(t *testing.T) {
	assert.Equal(t, lifecycle.MetadataKeyDockerNetwork, dockerNetworkProfileConfigKey)
	assert.Equal(t, lifecycle.MetadataKeyDockerNetworkGwPriority, dockerNetworkGwPriorityProfileConfigKey)
	assert.Equal(t, lifecycle.MetadataKeyDockerAdditionalNetworks, dockerAdditionalNetworksProfileConfigKey)
}
