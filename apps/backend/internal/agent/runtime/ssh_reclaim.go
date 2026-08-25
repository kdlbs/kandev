package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"golang.org/x/crypto/ssh"
)

// SSHTaskDirReclaimRequest describes the recorded SSH connection and remote
// path for one terminal task cleanup attempt.
type SSHTaskDirReclaimRequest struct {
	Host            string
	Port            int
	User            string
	HostFingerprint string
	IdentitySource  string
	IdentityFile    string
	ProxyJump       string
	Shell           string
	WorkdirRoot     string
	TaskDir         string
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
	logger        *logger.Logger
	resolveTarget func(lifecycle.SSHConnConfig) (*lifecycle.SSHTarget, error)
	dial          func(context.Context, *lifecycle.SSHTarget) (*ssh.Client, error)
	reclaim       func(context.Context, *ssh.Client, string, string, string) (lifecycle.SSHReclaimOutcome, lifecycle.SSHReclaimVerdict, error)
}

// NewSSHTaskDirReclaimer creates the production adapter over the lifecycle
// SSH target resolver, pinned dialer, and guarded task-directory reclaimer.
func NewSSHTaskDirReclaimer(log *logger.Logger) SSHTaskDirReclaimer {
	return &sshTaskDirReclaimer{
		logger:        log,
		resolveTarget: lifecycle.ResolveSSHTarget,
		dial:          lifecycle.DialSSH,
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
	resolveTarget := a.resolveTarget
	if resolveTarget == nil {
		resolveTarget = lifecycle.ResolveSSHTarget
	}
	target, err := resolveTarget(lifecycle.SSHConnConfig{
		Host:              req.Host,
		Port:              req.Port,
		User:              req.User,
		IdentitySource:    lifecycle.SSHIdentitySource(req.IdentitySource),
		IdentityFile:      req.IdentityFile,
		ProxyJump:         req.ProxyJump,
		PinnedFingerprint: req.HostFingerprint,
	})
	if err != nil {
		return SSHTaskDirReclaimResult{}, fmt.Errorf("resolve ssh target %s: %w", req.Host, err)
	}
	dial := a.dial
	if dial == nil {
		dial = lifecycle.DialSSH
	}
	client, err := dial(ctx, target)
	if err != nil {
		return SSHTaskDirReclaimResult{}, fmt.Errorf("dial ssh %s: %w", req.Host, err)
	}
	defer func() { _ = client.Close() }()

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
