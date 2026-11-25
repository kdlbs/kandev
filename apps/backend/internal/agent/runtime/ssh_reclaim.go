package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
)

// SSHTaskDirReclaimRequest describes the recorded SSH connection and remote
// path for one terminal task cleanup attempt.
type SSHTaskDirReclaimRequest struct {
	ConnectionID              string
	RouteStamp                string
	IdentityGeneration        string
	BastionIdentityGeneration string
	HostAlias                 string
	LegacyRouteRecorded       bool
	IdentityAgent             string
	Host                      string
	Port                      int
	User                      string
	HostFingerprint           string
	IdentitySource            string
	IdentityFile              string
	ProxyJump                 string
	Shell                     string
	WorkdirRoot               string
	TaskDir                   string
}

// SSHTaskDirReclaimResult reports whether the remote task directory was
// removed or deliberately preserved by a safety verdict.
type SSHTaskDirReclaimResult struct {
	Removed    bool
	SkipReason string
	Detail     string
}

// SSHTaskDirReclaimer is the runtime seam for one short-lived SSH reclamation
// connection. Higher-level packages depend on this contract instead of the
// lifecycle implementation details.
type SSHTaskDirReclaimer interface {
	ReclaimTaskDir(context.Context, SSHTaskDirReclaimRequest) (SSHTaskDirReclaimResult, error)
}

type sshTaskDirReclaimer struct {
	logger  *logger.Logger
	open    func(context.Context, SSHTaskDirReclaimRequest) (*ssh.Client, func(), error)
	reclaim func(context.Context, *ssh.Client, string, string, string) (lifecycle.SSHReclaimOutcome, lifecycle.SSHReclaimVerdict, error)
}

// NewSSHTaskDirReclaimer creates the production adapter over the lifecycle
// managed retained route opener and guarded task-directory reclaimer.
func NewSSHTaskDirReclaimer(log *logger.Logger, connections *sshconnections.Service) SSHTaskDirReclaimer {
	return &sshTaskDirReclaimer{
		logger: log,
		open: func(ctx context.Context, req SSHTaskDirReclaimRequest) (*ssh.Client, func(), error) {
			return openReclaimRoute(ctx, connections, req)
		},
		reclaim: func(ctx context.Context, client *ssh.Client, shell, workdirRoot, taskDir string) (lifecycle.SSHReclaimOutcome, lifecycle.SSHReclaimVerdict, error) {
			return lifecycle.NewSSHTaskDirReclaimer(client, shell, log).Reclaim(ctx, workdirRoot, taskDir)
		},
	}
}

func (a *sshTaskDirReclaimer) ReclaimTaskDir(
	ctx context.Context,
	req SSHTaskDirReclaimRequest,
) (SSHTaskDirReclaimResult, error) {
	// A launch cannot happen without a trusted host key, so an empty pinned
	// fingerprint means the recorded connection is not one Kandev established.
	// Dialling anyway would accept whatever key answered and then delete a
	// directory on it.
	if req.HostFingerprint == "" {
		return SSHTaskDirReclaimResult{}, errors.New(
			"ssh reclaim: no pinned host fingerprint recorded for " + req.Host)
	}
	if a.open == nil {
		return SSHTaskDirReclaimResult{}, sshconnections.ErrDisabled
	}
	client, cleanup, err := a.open(ctx, req)
	if err != nil {
		return SSHTaskDirReclaimResult{}, fmt.Errorf("dial ssh %s: %w", req.Host, err)
	}
	defer cleanup()

	reclaim := a.reclaim
	if reclaim == nil {
		reclaim = func(ctx context.Context, client *ssh.Client, shell, workdirRoot, taskDir string) (lifecycle.SSHReclaimOutcome, lifecycle.SSHReclaimVerdict, error) {
			return lifecycle.NewSSHTaskDirReclaimer(client, shell, a.logger).Reclaim(ctx, workdirRoot, taskDir)
		}
	}
	outcome, verdict, err := reclaim(ctx, client, req.Shell, req.WorkdirRoot, req.TaskDir)
	if err != nil {
		return SSHTaskDirReclaimResult{}, err
	}
	return SSHTaskDirReclaimResult{
		Removed:    outcome == lifecycle.SSHReclaimOutcomeRemoved,
		SkipReason: string(verdict.Reason),
		Detail:     verdict.Detail,
	}, nil
}

var _ SSHTaskDirReclaimer = (*sshTaskDirReclaimer)(nil)

func openReclaimRoute(ctx context.Context, connections *sshconnections.Service, req SSHTaskDirReclaimRequest) (*ssh.Client, func(), error) {
	if connections == nil {
		return nil, func() {}, sshconnections.ErrDisabled
	}
	if req.ConnectionID != "" {
		return connections.OpenRetainedRoute(ctx, sshconnections.RuntimeSnapshot{
			Route: sshconnections.Route{Stamp: req.RouteStamp, Destination: sshconnections.Connection{
				ID: req.ConnectionID, Host: req.Host, Port: req.Port, User: req.User, Fingerprint: req.HostFingerprint,
			}},
			IdentityGeneration: req.IdentityGeneration, BastionIdentityGeneration: req.BastionIdentityGeneration,
		})
	}
	if !req.LegacyRouteRecorded {
		return nil, func() {}, sshconnections.ErrInvalidRoute
	}
	client, err := connections.DialRetainedLegacy(ctx, sshtransport.SSHTarget{
		Host: req.Host, Port: req.Port, User: req.User, PinnedFingerprint: req.HostFingerprint,
		IdentitySource: sshtransport.SSHIdentitySource(req.IdentitySource), IdentityFile: req.IdentityFile,
		ProxyJump: req.ProxyJump, OriginalHost: req.HostAlias, IdentityAgent: req.IdentityAgent,
	})
	if err != nil {
		return nil, func() {}, err
	}
	return client, func() { _ = client.Close() }, nil
}
