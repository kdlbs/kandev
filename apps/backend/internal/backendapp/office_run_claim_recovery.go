package backendapp

import (
	"context"
	"errors"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type runClaimTaskSessionReader interface {
	ListTaskSessions(ctx context.Context, taskID string) ([]*taskmodels.TaskSession, error)
	GetLatestTurnBySessionID(ctx context.Context, sessionID string) (*taskmodels.Turn, error)
}

type runClaimRuntimeProbe interface {
	ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error)
}

type officeRunProcessorRuntime interface {
	runtimeapi.Backend
	runClaimRuntimeProbe
}

type officeRunClaimSessionProbe struct {
	sessions runClaimTaskSessionReader
	runtime  runClaimRuntimeProbe
}

var _ officeservice.RunClaimSessionProbe = (*officeRunClaimSessionProbe)(nil)

func (p *officeRunClaimSessionProbe) FindTaskSessionsForClaim(
	ctx context.Context, taskID, agentProfileID, persistedSessionID string, claimedAt time.Time,
) ([]officeservice.RunClaimSessionEvidence, error) {
	if p.sessions == nil {
		return nil, errors.New("task session repository is unavailable")
	}
	sessions, err := p.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	candidates, err := taskSessionCandidates(sessions, taskID, agentProfileID, persistedSessionID)
	if err != nil {
		return nil, err
	}
	evidence := make([]officeservice.RunClaimSessionEvidence, 0, len(candidates))
	for _, session := range candidates {
		turn, err := p.sessions.GetLatestTurnBySessionID(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		turnStartedAfterClaim := turn != nil && !turn.StartedAt.Before(claimedAt)
		if persistedSessionID != "" || turnStartedAfterClaim || taskSessionMayOwnClaim(session.State) {
			evidence = append(evidence, officeservice.RunClaimSessionEvidence{
				SessionID: session.ID, TurnStartedAfterClaim: turnStartedAfterClaim,
			})
		}
	}
	return evidence, nil
}

func taskSessionCandidates(
	sessions []*taskmodels.TaskSession, taskID, agentProfileID, persistedSessionID string,
) ([]*taskmodels.TaskSession, error) {
	var candidates []*taskmodels.TaskSession
	persistedSessionFound := persistedSessionID == ""
	for _, session := range sessions {
		if session == nil || session.ID == "" || session.TaskID != taskID {
			continue
		}
		if persistedSessionID != "" {
			if session.ID != persistedSessionID {
				continue
			}
			if session.AgentProfileID != agentProfileID {
				return nil, errors.New("persisted run session does not belong to its task agent")
			}
			persistedSessionFound = true
			candidates = append(candidates, session)
			break
		}
		if session.AgentProfileID == agentProfileID {
			candidates = append(candidates, session)
		}
	}
	if !persistedSessionFound {
		return nil, errors.New("persisted run task session is missing")
	}
	return candidates, nil
}

func taskSessionMayOwnClaim(state taskmodels.TaskSessionState) bool {
	switch state {
	case taskmodels.TaskSessionStateStarting, taskmodels.TaskSessionStateRunning,
		taskmodels.TaskSessionStateIdle, taskmodels.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
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
