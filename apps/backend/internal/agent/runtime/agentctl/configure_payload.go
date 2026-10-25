package client

import (
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sandbox"
)

type agentConfigurePayload struct {
	Sandbox         *sandbox.Launch    `json:"sandbox,omitempty"`
	LaunchWrapper   launchwrapper.Spec `json:"launch_wrapper,omitzero"`
	Command         string             `json:"command"`
	AgentArgs       *[]string          `json:"agent_args,omitempty"`
	ContinueCommand string             `json:"continue_command,omitempty"`
	ContinueArgs    *[]string          `json:"continue_args,omitempty"`
	Env             map[string]string  `json:"env,omitempty"`
	ReplaceEnv      bool               `json:"replace_env,omitempty"`
}
