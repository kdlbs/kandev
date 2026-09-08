package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/agents"
)

// fakeIdentityAgent is an agents.Agent whose identity, command, runtime config
// and native-binary preference are all set by the test. It embeds MockAgent so
// only the behaviour under test has to be spelled out.
type fakeIdentityAgent struct {
	*agents.MockAgent
	id            string
	name          string
	installScript string
	command       agents.Command
}

func newFakeIdentityAgent(id string) *fakeIdentityAgent {
	return &fakeIdentityAgent{MockAgent: agents.NewMockAgent(), id: id}
}

func (a *fakeIdentityAgent) ID() string   { return a.id }
func (a *fakeIdentityAgent) Name() string { return a.name }

func (a *fakeIdentityAgent) InstallScript() string { return a.installScript }

func (a *fakeIdentityAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	if len(a.command.Args()) > 0 {
		return a.command
	}
	return a.MockAgent.BuildCommand(opts)
}

// fakeRuntimeAgent overrides only the runtime config fields the remote
// create-instance request reads.
type fakeRuntimeAgent struct {
	*agents.MockAgent
	id                 string
	requiresKill       bool
	stripEnv           []string
	namespacesMCPTools bool
}

func (a *fakeRuntimeAgent) ID() string { return a.id }

func (a *fakeRuntimeAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{
		RequiresProcessKill:        a.requiresKill,
		StripEnv:                   a.stripEnv,
		NamespacesMCPToolsByServer: a.namespacesMCPTools,
	}
}

// fakeNativeBinaryAgent additionally declares a standalone CLI, exercising the
// NativeBinaryAgent preference path in the SSH agent-binary preflight.
type fakeNativeBinaryAgent struct {
	*fakeIdentityAgent
	nativeBinary string
}

func (a *fakeNativeBinaryAgent) NativeBinaryName() string { return a.nativeBinary }

var (
	_ agents.Agent             = (*fakeIdentityAgent)(nil)
	_ agents.Agent             = (*fakeRuntimeAgent)(nil)
	_ agents.NativeBinaryAgent = (*fakeNativeBinaryAgent)(nil)
)
