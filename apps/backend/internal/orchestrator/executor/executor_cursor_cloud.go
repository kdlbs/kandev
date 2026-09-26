package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (e *Executor) launchCursorCloudSession(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	agentProfileID, prompt string,
	opts LaunchOptions,
	execCfg executorConfig,
) (*TaskExecution, error) {
	if !opts.StartAgent {
		return newUnstartedCursorCloudExecution(task, session, agentProfileID), nil
	}
	if err := validateCursorCloudLaunchOptions(opts); err != nil {
		return nil, err
	}
	return e.startCursorCloudExecution(ctx, task, session, agentProfileID, prompt, opts, execCfg)
}

func newUnstartedCursorCloudExecution(task *v1.Task, session *models.TaskSession, agentProfileID string) *TaskExecution {
	return &TaskExecution{
		TaskID: task.ID, AgentProfileID: agentProfileID, StartedAt: session.StartedAt,
		SessionState: v1.TaskSessionStateCreated, LastUpdate: time.Now().UTC(), SessionID: session.ID,
	}
}

func validateCursorCloudLaunchOptions(opts LaunchOptions) error {
	if len(opts.Attachments) != 0 {
		return errors.New("cursor cloud does not support prompt attachments")
	}
	unsupported := [...]bool{
		opts.RouteOverride != nil, opts.OfficeAgentProfileID != "", len(opts.Env) != 0,
		len(opts.AdditionalSkillSlugs) != 0,
	}
	for _, option := range unsupported {
		if option {
			return errors.New("cursor cloud does not support routed execution options")
		}
	}
	return nil
}

func (e *Executor) startCursorCloudExecution(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	agentProfileID, prompt string,
	opts LaunchOptions,
	execCfg executorConfig,
) (*TaskExecution, error) {
	prompt = e.injectHandoverIfNeeded(ctx, task.ID, session.ID, prompt)
	if opts.McpMode == "" {
		mode, err := e.resolveTaskSessionMCPMode(ctx, task.ID, session, true)
		if err != nil {
			return nil, err
		}
		opts.McpMode = mode
	}
	if opts.McpProfile == nil {
		resolved, err := e.resolveTaskSessionMCPProfile(ctx, task.ID, session, true)
		if err != nil {
			return nil, err
		}
		opts.McpProfile = &resolved
	}
	model, _ := session.AgentProfileSnapshot["model"].(string)
	req := &LaunchAgentRequest{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, SessionID: session.ID,
		TaskTitle: task.Title, AgentProfileID: agentProfileID, TurnID: opts.TurnID,
		StartAgent: true, TaskDescription: prompt, Priority: task.Priority,
		ExecutorType: execCfg.ExecutorType, ExecutorConfig: execCfg.ExecutorCfg,
		Metadata: cloneMetadata(execCfg.Metadata), ModelOverride: strings.TrimSpace(model),
		McpMode: opts.McpMode, McpProfile: opts.McpProfile, AutoCreatePR: opts.AutoCreatePR,
	}
	if execCfg.ExecutorID != "" {
		session.ExecutorID = execCfg.ExecutorID
	}
	response, err := e.agentManager.LaunchAgent(ctx, req)
	if err != nil {
		return nil, e.handleLaunchFailure(ctx, task.ID, session.ID, "", "", err)
	}
	if response == nil || response.AgentExecutionID == "" {
		return nil, e.handleLaunchFailure(ctx, task.ID, session.ID, "", "", fmt.Errorf("cursor cloud launch returned no execution identity"))
	}
	if opts.OnExecutionAdmitted != nil {
		opts.OnExecutionAdmitted(response.AgentExecutionID)
	}
	return e.finalizeLaunch(ctx, task, session, agentProfileID, session.ID, &repoInfo{}, response, true, execCfg)
}
