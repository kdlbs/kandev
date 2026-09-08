package acp

import (
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp/backgroundlaunch"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func init() {
	backgroundlaunch.Register(claudeBackgroundLaunchRecognizer{})
}

// claudeBackgroundLaunchRecognizer implements backgroundlaunch.Recognizer
// for Claude: a shell_exec tool call is a detached background launch when
// the agent reports Background=true (spec D7's one shipped recogniser,
// reproducing the condition stampBackgroundShellWork checked inline before
// this seam existed).
type claudeBackgroundLaunchRecognizer struct{}

func (claudeBackgroundLaunchRecognizer) AgentID() string { return claudeAgentID }

func (claudeBackgroundLaunchRecognizer) RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool {
	return payload != nil && payload.ShellExec() != nil && payload.ShellExec().Background
}
