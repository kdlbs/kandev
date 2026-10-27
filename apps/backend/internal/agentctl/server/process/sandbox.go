package process

import (
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sandbox"
)

// ExecutionConfiguration is an authenticated, atomic process configuration.
type ExecutionConfiguration struct {
	Command             string
	Args                []string
	ArgsPresent         bool
	Environment         map[string]string
	ContinueCommand     string
	ContinueArgs        []string
	ContinueArgsPresent bool
	ReplaceEnvironment  bool
	Wrapper             launchwrapper.Spec
	Sandbox             *sandbox.Launch
}

func (m *Manager) ConfigureExecution(request ExecutionConfiguration) error {
	return m.configure(request.Command, request.Args, request.ArgsPresent, request.Environment,
		request.ContinueCommand, request.ContinueArgs, request.ContinueArgsPresent, request.ReplaceEnvironment,
		request.Sandbox, request.Wrapper)
}

func (m *Manager) validateSandboxConfiguration(launch *sandbox.Launch, argsPresent bool) error {
	if launch == nil {
		if m.cfg.SandboxRequired || m.cfg.Sandbox != nil {
			return sandbox.ErrInvalidPolicy
		}
		return nil
	}
	if !argsPresent {
		return sandbox.ErrInvalidPolicy
	}
	return launch.Validate()
}

func (m *Manager) SandboxOwner() (*sandbox.ProcessOwner, error) {
	return m.sandboxOwner.Load().LiveSnapshot()
}

func (r *InteractiveRunner) SandboxOwner(processID string) (*sandbox.ProcessOwner, error) {
	proc, ok := r.get(processID)
	if !ok {
		return nil, sandbox.ErrInvalidPolicy
	}
	proc.mu.Lock()
	defer proc.mu.Unlock()
	return proc.sandboxOwner.LiveSnapshot()
}

func (m *Manager) sandboxLogArgs(args []string) []string {
	if m.cfg.Sandbox != nil {
		return []string{"<sandboxed arguments omitted>"}
	}
	return args
}
