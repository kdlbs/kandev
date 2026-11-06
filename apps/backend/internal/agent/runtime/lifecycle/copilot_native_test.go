package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestCopilotNativeSessionEventsForLaunch(t *testing.T) {
	if !copilotNativeSessionEventsForLaunch(agents.CopilotACPAgentID, "stream-1") {
		t.Fatal("durable copilot launch should enable native events")
	}
	if copilotNativeSessionEventsForLaunch("claude-acp", "stream-1") {
		t.Fatal("other providers must not enable copilot native events")
	}
	if copilotNativeSessionEventsForLaunch(agents.CopilotACPAgentID, "") {
		t.Fatal("a launch without durable delivery must not enable native events")
	}
}

type copilotCapableExecutor struct {
	ExecutorBackend
	capable bool
}

func (c copilotCapableExecutor) SupportsCopilotNativeSessionEvents() bool { return c.capable }

func TestStandaloneExecutorCopilotNativeCapability(t *testing.T) {
	fresh := &StandaloneExecutor{}
	if !fresh.SupportsCopilotNativeSessionEvents() {
		t.Fatal("a fresh same-version standalone peer should support native events")
	}
	oldPeer := &StandaloneExecutor{peerCapabilitiesSet: true}
	if oldPeer.SupportsCopilotNativeSessionEvents() {
		t.Fatal("an adopted peer without the capability must stay unavailable")
	}
	currentPeer := &StandaloneExecutor{
		peerCapabilitiesSet: true,
		peerCapabilities:    []string{acpcompat.CopilotNativeCompactionCapability},
	}
	if !currentPeer.SupportsCopilotNativeSessionEvents() {
		t.Fatal("an adopted peer advertising the capability should support native events")
	}
}

func TestApplyCopilotNativeLaunchCapability(t *testing.T) {
	managerWithProjector := &Manager{streamManager: &StreamManager{
		delivery: &canonicalProjectionDeliveryRepository{},
	}}
	managerWithoutProjector := &Manager{streamManager: &StreamManager{}}
	execution := func(agentID string, deliveryStreamID string) *AgentExecution {
		return &AgentExecution{AgentID: agentID, DeliveryStreamID: deliveryStreamID}
	}

	copilot := execution(agents.CopilotACPAgentID, "stream-1")
	managerWithProjector.applyCopilotNativeLaunchCapability(copilot, copilotCapableExecutor{capable: true}, "stream-1")
	if !copilot.CopilotNativeSessionEvents {
		t.Fatal("capable peer with a projector should enable native events")
	}

	oldPeer := execution(agents.CopilotACPAgentID, "stream-1")
	managerWithProjector.applyCopilotNativeLaunchCapability(oldPeer, copilotCapableExecutor{capable: false}, "stream-1")
	if oldPeer.CopilotNativeSessionEvents {
		t.Fatal("peer without the capability must not enable native events")
	}

	noProjector := execution(agents.CopilotACPAgentID, "stream-1")
	managerWithoutProjector.applyCopilotNativeLaunchCapability(noProjector, copilotCapableExecutor{capable: true}, "stream-1")
	if noProjector.CopilotNativeSessionEvents {
		t.Fatal("missing canonical projector must not enable native events")
	}

	otherProvider := execution("claude-acp", "stream-1")
	managerWithProjector.applyCopilotNativeLaunchCapability(otherProvider, copilotCapableExecutor{capable: true}, "stream-1")
	if otherProvider.CopilotNativeSessionEvents {
		t.Fatal("other providers must not enable Copilot native events")
	}
}

func TestCopilotNativeCapabilityCoversEveryAgentctlExecutor(t *testing.T) {
	executors := []interface{ SupportsCopilotNativeSessionEvents() bool }{
		&DockerExecutor{},
		&RemoteDockerExecutor{},
		&KubernetesExecutor{},
		&SSHExecutor{},
		&SpritesExecutor{},
		&PluginRemoteExecutor{},
	}
	for _, executor := range executors {
		if !executor.SupportsCopilotNativeSessionEvents() {
			t.Fatalf("%T does not propagate the native-event opt-in", executor)
		}
	}
}
