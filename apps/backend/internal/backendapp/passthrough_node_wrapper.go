package backendapp

import (
	"slices"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// configurePassthroughRunner fails the backend's own passthrough runner closed.
// It runs inside the backend process and cannot apply an agentctl node wrapper.
// The only host-visible signal is the adopted agentctl advertising the node
// wrapper capability, which means the operator wants agents on this node wrapped.
func configurePassthroughRunner(runner *process.InteractiveRunner, peerCapabilities []string) {
	if slices.Contains(peerCapabilities, launchwrapper.NodeCapability) {
		runner.SetNodeLaunchWrapper(launchwrapper.Node{Required: true})
	}
}
