package backendapp

import (
	"context"
	"errors"

	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type runClaimTaskSessionReader interface {
	ListTaskSessions(ctx context.Context, taskID string) ([]*taskmodels.TaskSession, error)
}

type runClaimRuntimeProbe interface {
	ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error)
}

type officeRunClaimSessionProbe struct {
	sessions runClaimTaskSessionReader
	runtime  runClaimRuntimeProbe
}

var _ officeservice.RunClaimSessionProbe = (*officeRunClaimSessionProbe)(nil)

func (p *officeRunClaimSessionProbe) FindActiveTaskSession(
	ctx context.Context, taskID, agentProfileID string,
) (string, error) {
	if p.sessions == nil {
		return "", errors.New("task session repository is unavailable")
	}
	sessions, err := p.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", err
	}
	for _, session := range sessions {
		if session == nil || session.ID == "" || session.AgentProfileID != agentProfileID {
			continue
		}
		if session.State == taskmodels.TaskSessionStateStarting || session.State == taskmodels.TaskSessionStateRunning {
			return session.ID, nil
		}
		// A ready task session with an execution identity may have launched
		// this run even if best-effort persistence of runs.session_id failed.
		// Its delayed Office completion still owns settlement of the claim.
		if session.State == taskmodels.TaskSessionStateWaitingForInput && session.AgentExecutionID != "" {
			return session.ID, nil
		}
	}
	return "", nil
}

func (p *officeRunClaimSessionProbe) ProbeAgentRunningForSession(
	ctx context.Context, sessionID string,
) (bool, error) {
	if p.runtime == nil {
		return false, errors.New("agent runtime liveness probe is unavailable")
	}
	return p.runtime.ProbeAgentRunningForSession(ctx, sessionID)
}

func wireOfficeRunClaimSessionProbe(
	svc *officeservice.Service, sessions runClaimTaskSessionReader, runtime runClaimRuntimeProbe,
) {
	svc.SetRunClaimSessionProbe(&officeRunClaimSessionProbe{sessions: sessions, runtime: runtime})
}
