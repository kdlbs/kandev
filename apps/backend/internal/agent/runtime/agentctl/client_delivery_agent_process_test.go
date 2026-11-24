package client

import (
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/stretchr/testify/require"
)

func TestDeliveryDescriptorOwnsAgentProcessIdentity(t *testing.T) {
	c := NewClient("127.0.0.1", 1, newTestLogger())
	identity := processidentity.Identity{PID: 42, GroupID: 42, BirthToken: "birth"}
	status := &DeliveryStatus{AgentProcess: &identity}
	c.setDeliveryStatus(status)

	identity.BirthToken = "changed input"
	cached, ok := c.DeliveryRecoveryDescriptor()
	require.True(t, ok)
	require.Equal(t, "birth", cached.AgentProcess.BirthToken)

	cached.AgentProcess.BirthToken = "changed output"
	again, ok := c.DeliveryRecoveryDescriptor()
	require.True(t, ok)
	require.Equal(t, "birth", again.AgentProcess.BirthToken)

	c.setDeliveryStatus(&DeliveryStatus{})
	legacy, ok := c.DeliveryRecoveryDescriptor()
	require.True(t, ok)
	require.Nil(t, legacy.AgentProcess)
}
