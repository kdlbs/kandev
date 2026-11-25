package runtime

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"golang.org/x/crypto/ssh"
)

// ManagedSSHConnections is the runtime contract for retained managed SSH routes.
type ManagedSSHConnections = lifecycle.ManagedSSHConnections

// ClassifySSHDialError exposes the closed runtime SSH failure classification.
func ClassifySSHDialError(err error) SSHReachabilityReason {
	return SSHReachabilityReason(lifecycle.ClassifyDialError(err))
}

// CheckSSHRemotePlatform verifies that Kandev can run a workload on the connected host.
func CheckSSHRemotePlatform(ctx context.Context, client *ssh.Client) error {
	info, err := lifecycle.SSHProbeRemote(ctx, client)
	if err != nil {
		return err
	}
	return lifecycle.SSHRequireSupportedRemotePlatform(info.Platform)
}

// IsSSHRuntimeRouteMetadataKey identifies settings owned by an SSH executor's connection.
func IsSSHRuntimeRouteMetadataKey(key string) bool {
	return lifecycle.IsSSHRuntimeRouteMetadataKey(key)
}
