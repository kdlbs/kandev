package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/githubauth"
)

var ErrGitHubCredentialBrokerUnreachable = errors.New("GitHub credential broker is unreachable from executor")

const brokerReachabilityScript = `if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to reach the Kandev GitHub credential broker" >&2
  exit 69
fi
broker_status=$(curl -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%{http_code}' "$KANDEV_GITHUB_CREDENTIAL_BROKER_URL") || exit 69
if [ "$broker_status" != "204" ]; then
  echo "Kandev GitHub credential broker readiness returned HTTP $broker_status" >&2
  exit 69
fi`

type brokerCommandRunner func(context.Context, string, map[string]string) ([]byte, error)

func runBrokerReachabilityPreflight(
	ctx context.Context,
	env map[string]string,
	run brokerCommandRunner,
) error {
	if !hasManagedGitHubBrokerEnv(env) {
		return nil
	}
	probeEnv := map[string]string{
		githubauth.CredentialBrokerURLEnv: env[githubauth.CredentialBrokerURLEnv],
	}
	output, err := run(ctx, brokerReachabilityScript, probeEnv)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrGitHubCredentialBrokerUnreachable,
			strings.TrimSpace(string(output)), err)
	}
	return nil
}

type brokerAgentctlProcessClient interface {
	StartProcess(context.Context, agentctl.StartProcessRequest) (*agentctl.ProcessInfo, error)
	GetProcess(context.Context, string, bool) (*agentctl.ProcessInfo, error)
}

func runBrokerReachabilityViaAgentctl(
	ctx context.Context,
	client brokerAgentctlProcessClient,
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
		return waitBrokerReachabilityProcess(ctx, client, process)
	})
}

func waitBrokerReachabilityProcess(
	ctx context.Context,
	client brokerAgentctlProcessClient,
	process *agentctl.ProcessInfo,
) ([]byte, error) {
	if process == nil {
		return nil, errors.New("agentctl returned no process")
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if output, done, err := brokerProcessResult(process); done {
			return output, err
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-deadline.C:
			return nil, errors.New("broker reachability process timed out")
		case <-ticker.C:
			var err error
			process, err = client.GetProcess(ctx, process.ID, true)
			if err != nil {
				return nil, err
			}
		}
	}
}

func brokerProcessResult(process *agentctl.ProcessInfo) ([]byte, bool, error) {
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
		return []byte(output.String()), true, fmt.Errorf("probe process status %s", process.Status)
	}
	return []byte(output.String()), true, nil
}
