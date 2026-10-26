package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/githubauth"
)

var ErrGitCredentialBrokerUnreachable = errors.New("git credential broker is unreachable from executor")

// ErrGitHubCredentialBrokerUnreachable remains an alias for callers which
// predate provider-neutral leases.
var ErrGitHubCredentialBrokerUnreachable = ErrGitCredentialBrokerUnreachable

const brokerReachabilityScript = `if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to reach the Kandev Git credential broker" >&2
  exit 69
fi
broker_status=$(curl -sS --connect-timeout 5 --max-time "${KANDEV_BROKER_PROBE_MAX_TIME:-10}" -o /dev/null -w '%{http_code}' "$KANDEV_GITHUB_CREDENTIAL_BROKER_URL") || exit 69
if [ "$broker_status" != "204" ]; then
  echo "Kandev Git credential broker readiness returned HTTP $broker_status" >&2
  exit 69
fi`

// brokerProbeMaxTimeEnv overrides the probe's curl limit. It is set only for
// the loopback relay, whose upstream forward alone may take 15 seconds.
const brokerProbeMaxTimeEnv = "KANDEV_BROKER_PROBE_MAX_TIME"

const (
	defaultProbeWait     = 15 * time.Second
	relayProbeCurlLimit  = 20 * time.Second
	relayProbeWaitLimit  = 25 * time.Second
	loopbackRelayURLHead = "http://127.0.0.1:"
)

// isBrokerRelayURL reports whether rawURL is an instance's loopback Git
// credential relay, which the probe must give more time than a direct broker.
func isBrokerRelayURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, loopbackRelayURLHead) && strings.HasSuffix(rawURL, agentctlconfig.BrokerRelayPath)
}

func probeWait(env map[string]string) time.Duration {
	if isBrokerRelayURL(env[githubauth.CredentialBrokerURLEnv]) {
		return relayProbeWaitLimit
	}
	return defaultProbeWait
}

type brokerCommandRunner func(context.Context, string, map[string]string) ([]byte, error)

func runBrokerReachabilityPreflight(
	ctx context.Context,
	env map[string]string,
	run brokerCommandRunner,
) error {
	if !hasManagedGitCredentialBrokerEnv(env) {
		return nil
	}
	probeEnv := map[string]string{
		githubauth.CredentialBrokerURLEnv: env[githubauth.CredentialBrokerURLEnv],
	}
	if isBrokerRelayURL(probeEnv[githubauth.CredentialBrokerURLEnv]) {
		probeEnv[brokerProbeMaxTimeEnv] = strconv.Itoa(int(relayProbeCurlLimit / time.Second))
	}
	output, err := run(ctx, brokerReachabilityScript, probeEnv)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrGitCredentialBrokerUnreachable,
			strings.TrimSpace(string(output)), err)
	}
	return nil
}

type agentctlProcessClient interface {
	StartProcess(context.Context, agentctl.StartProcessRequest) (*agentctl.ProcessInfo, error)
	GetProcess(context.Context, string, bool) (*agentctl.ProcessInfo, error)
}

func runBrokerReachabilityViaAgentctl(
	ctx context.Context,
	client agentctlProcessClient,
	sessionID string,
	env map[string]string,
) error {
	return runBrokerReachabilityPreflight(ctx, env, func(
		ctx context.Context,
		command string,
		probeEnv map[string]string,
	) ([]byte, error) {
		process, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
			SessionID: sessionID,
			Kind:      agentctltypes.ProcessKindCustom,
			Command:   command,
			Env:       probeEnv,
		})
		if err != nil {
			return nil, err
		}
		return waitAgentctlProcess(ctx, client, process, probeWait(probeEnv))
	})
}

// waitAgentctlProcess polls an agentctl process until it ends or timeout elapses,
// and returns its output. A non-zero exit is an *agentctlProcessError.
func waitAgentctlProcess(
	ctx context.Context,
	client agentctlProcessClient,
	process *agentctl.ProcessInfo,
	timeout time.Duration,
) ([]byte, error) {
	if process == nil {
		return nil, errors.New("agentctl returned no process")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if output, done, err := agentctlProcessResult(process); done {
			return output, err
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-deadline.C:
			return nil, errors.New("agentctl process timed out")
		case <-ticker.C:
			var err error
			process, err = client.GetProcess(ctx, process.ID, true)
			if err != nil {
				return nil, err
			}
		}
	}
}

func agentctlProcessResult(process *agentctl.ProcessInfo) ([]byte, bool, error) {
	if process.Status != agentctltypes.ProcessStatusExited &&
		process.Status != agentctltypes.ProcessStatusFailed &&
		process.Status != agentctltypes.ProcessStatusStopped {
		return nil, false, nil
	}
	var output strings.Builder
	for _, chunk := range process.Output {
		output.WriteString(chunk.Data)
	}
	if process.ExitCode == nil || *process.ExitCode != 0 || process.Status != agentctltypes.ProcessStatusExited {
		return []byte(output.String()), true, &agentctlProcessError{status: process.Status, exitCode: process.ExitCode}
	}
	return []byte(output.String()), true, nil
}

type agentctlProcessError struct {
	status   agentctltypes.ProcessStatus
	exitCode *int
}

func (e *agentctlProcessError) Error() string {
	if e.exitCode == nil {
		return fmt.Sprintf("process status %s", e.status)
	}
	return fmt.Sprintf("process status %s, exit code %d", e.status, *e.exitCode)
}
