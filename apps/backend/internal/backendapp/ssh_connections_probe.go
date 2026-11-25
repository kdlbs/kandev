package backendapp

import (
	"context"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	reachabilitypkg "github.com/kandev/kandev/internal/executors/reachability"
	"github.com/kandev/kandev/internal/sshconnections"
	"time"
)

func managedSSHProbe(services *Services) reachabilitypkg.ManagedProbe {
	if services.SSHConnections == nil {
		return nil
	}
	return func(ctx context.Context, id string) agentruntime.SSHProbeOutcome {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_, target, cleanup, err := services.SSHConnections.OpenExecutorProbe(probeCtx, id)
		if err == nil {
			defer cleanup()
			return agentruntime.SSHProbeOutcome{Success: true, Host: target.Host}
		}
		if ctx.Err() != nil {
			return agentruntime.SSHProbeOutcome{Cancelled: true}
		}
		reason := agentruntime.ClassifySSHDialError(err)
		var failure *sshconnections.ProbeFailure
		if errors.As(err, &failure) {
			switch failure.Code {
			case "ssh_host_key_changed":
				reason = agentruntime.SSHReachabilityReasonHostKey
			case "ssh_authentication_rejected":
				reason = agentruntime.SSHReachabilityReasonAuth
			}
		}
		if errors.Is(err, sshconnections.ErrInvalidRoute) || errors.Is(err, sshconnections.ErrCredential) || errors.Is(err, sshconnections.ErrDisabled) {
			reason = agentruntime.SSHReachabilityReasonConfig
		}
		return agentruntime.SSHProbeOutcome{Reason: reason, Message: agentruntime.SanitizeSSHReachabilityMessage(err)}
	}
}
