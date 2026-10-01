package coordinator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/agentprofile"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// Runs-with sources. The first three name the agent the board already gives
// the task; coordinator is the coordinator's pair, added at approve.
const (
	RunsWithSourceStep        = "step"
	RunsWithSourceWorkflow    = "workflow"
	RunsWithSourceWorkspace   = "workspace"
	RunsWithSourceCoordinator = "coordinator"
	RunsWithSourceNone        = "none"
)

// RunsWith is the agent a proposal's task will run with, as the proposal read
// and list routes report it.
type RunsWith struct {
	AgentProfileID   string `json:"agent_profile_id"`
	AgentProfileName string `json:"agent_profile_name"`
	Source           string `json:"source"`
}

// Approval refusal texts (AC-COORDINATOR-CREATED-TASK-AGENT-001.10).
const (
	refusalAgentMissing     = "Agent for created tasks is not usable: the agent profile was removed."
	refusalAgentPassthrough = "Agent for created tasks is not usable: the agent profile uses CLI passthrough, which created tasks cannot use here."
	refusalExecutorMissing  = "Agent for created tasks is not usable: the executor was removed."
)

// taskAgentOutcome is what the chain decides for a task that does not exist
// yet. At most one of the flag fields is set.
type taskAgentOutcome struct {
	Source    string
	ProfileID string
	// Meta holds the task metadata the approval adds, nil for none.
	Meta map[string]any
	// Refusal is the proposal error text when the coordinator's pair is the
	// source and is not usable.
	Refusal string
	// StepGone, WorkspaceGone and CoordinatorGone report a row the chain
	// needs that no longer exists.
	StepGone, WorkspaceGone, CoordinatorGone bool
}

// taskAgentReads caches the reads of one request so a list over one workflow
// reads each step, default and profile once. A read error is cached too.
type taskAgentReads struct {
	s *Service

	steps     map[string]stepRead
	workflows map[string]stringRead
	workspace map[string]workspaceRead
	coords    map[string]coordRead
	agent     map[string]statusRead
	executor  map[string]statusRead
	names     map[string]string
}

type stepRead struct {
	step *wfmodels.WorkflowStep
	err  error
}
type stringRead struct {
	v   string
	err error
}
type workspaceRead struct {
	profile string
	gone    bool
	err     error
}
type coordRead struct {
	c   *Coordinator
	err error
}
type statusRead struct {
	status ProfileStatus
	err    error
}

func (s *Service) newTaskAgentReads() *taskAgentReads {
	return &taskAgentReads{
		s: s, steps: map[string]stepRead{}, workflows: map[string]stringRead{},
		workspace: map[string]workspaceRead{}, coords: map[string]coordRead{},
		agent: map[string]statusRead{}, executor: map[string]statusRead{}, names: map[string]string{},
	}
}

func (r *taskAgentReads) step(ctx context.Context, id string) (*wfmodels.WorkflowStep, error) {
	if c, ok := r.steps[id]; ok {
		return c.step, c.err
	}
	step, err := r.s.decisionTasks.GetWorkflowStep(ctx, id)
	if err == nil && step == nil {
		err = wfmodels.ErrWorkflowStepNotFound
	}
	r.steps[id] = stepRead{step, err}
	return step, err
}

func (r *taskAgentReads) workflowDefault(ctx context.Context, id string) (string, error) {
	if c, ok := r.workflows[id]; ok {
		return c.v, c.err
	}
	var v string
	wf, err := r.s.decisionTasks.GetWorkflow(ctx, id)
	switch {
	case errors.Is(err, repoerrors.ErrWorkflowNotFound):
		err = nil
	case err == nil && wf != nil:
		v = wf.AgentProfileID
	}
	r.workflows[id] = stringRead{v, err}
	return v, err
}

// workspaceDefault returns the workspace's default agent profile id, empty
// when unset, missing from the workspace, or CLI passthrough.
func (r *taskAgentReads) workspaceDefault(ctx context.Context, workspaceID string) workspaceRead {
	if c, ok := r.workspace[workspaceID]; ok {
		return c
	}
	var out workspaceRead
	ws, err := r.s.decisionTasks.GetWorkspace(ctx, workspaceID)
	switch {
	case errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		out.gone = true
	case err != nil:
		out.err = err
	case ws != nil && ws.DefaultAgentProfileID != nil && *ws.DefaultAgentProfileID != "":
		status, serr := r.agentStatus(ctx, workspaceID, *ws.DefaultAgentProfileID)
		switch {
		case serr != nil:
			out.err = serr
		case status == ProfileStatusOK:
			out.profile = *ws.DefaultAgentProfileID
		}
	}
	r.workspace[workspaceID] = out
	return out
}

func (r *taskAgentReads) coordinator(ctx context.Context, workspaceID, id string) (*Coordinator, error) {
	key := workspaceID + "/" + id
	if c, ok := r.coords[key]; ok {
		return c.c, c.err
	}
	c, err := r.s.store.GetCoordinator(ctx, workspaceID, id)
	r.coords[key] = coordRead{c, err}
	return c, err
}

func (r *taskAgentReads) agentStatus(ctx context.Context, workspaceID, id string) (ProfileStatus, error) {
	key := workspaceID + "/" + id
	if c, ok := r.agent[key]; ok {
		return c.status, c.err
	}
	st, err := r.s.validator.agentProfileStatus(ctx, workspaceID, id)
	r.agent[key] = statusRead{st, err}
	return st, err
}

func (r *taskAgentReads) executorStatus(ctx context.Context, id string) (ProfileStatus, error) {
	if c, ok := r.executor[id]; ok {
		return c.status, c.err
	}
	st, err := r.s.validator.executorProfileStatus(ctx, id)
	r.executor[id] = statusRead{st, err}
	return st, err
}

// profileName is the agent profile's name, or the id when it cannot be read.
func (r *taskAgentReads) profileName(ctx context.Context, id string) string {
	if n, ok := r.names[id]; ok {
		return n
	}
	name := id
	if p, err := r.s.validator.agents.GetAgentProfile(ctx, id); err == nil && p != nil && p.Name != "" {
		name = p.Name
	}
	r.names[id] = name
	return name
}

// taskAgent decides the agent of the task a create-task proposal with spec
// would make, through agentprofile.Resolve and the addition table. A returned
// error is a failed read.
func (r *taskAgentReads) taskAgent(ctx context.Context, workspaceID, coordinatorID string, spec ProposalSpec, startsAgent bool) (taskAgentOutcome, error) {
	step, err := r.step(ctx, spec.StepID)
	if errors.Is(err, wfmodels.ErrWorkflowStepNotFound) {
		return taskAgentOutcome{StepGone: true}, nil
	}
	if err != nil {
		return taskAgentOutcome{}, fmt.Errorf("read workflow step: %w", err)
	}
	in := agentprofile.Input{HasStep: true, StepSessionTarget: step.SessionTarget != nil}
	if !in.StepSessionTarget {
		in.StepProfile = step.AgentProfileID
		if in.StepProfile == "" {
			if in.WorkflowDefault, err = r.workflowDefault(ctx, spec.WorkflowID); err != nil {
				return taskAgentOutcome{}, fmt.Errorf("read workflow default: %w", err)
			}
		}
	}
	if id, src := agentprofile.Resolve(in); src != agentprofile.SourceNone {
		return taskAgentOutcome{Source: string(src), ProfileID: id}, nil
	}
	ws := r.workspaceDefault(ctx, workspaceID)
	switch {
	case ws.gone:
		return taskAgentOutcome{WorkspaceGone: true}, nil
	case ws.err != nil:
		return taskAgentOutcome{}, fmt.Errorf("read workspace default: %w", ws.err)
	}
	in.WorkspaceDefault = ws.profile
	if id, src := agentprofile.Resolve(in); src != agentprofile.SourceNone {
		out := taskAgentOutcome{Source: RunsWithSourceWorkspace, ProfileID: id}
		if startsAgent {
			out.Meta = map[string]any{taskmodels.MetaKeyAgentProfileID: id}
		}
		return out, nil
	}
	return r.coordinatorPair(ctx, workspaceID, coordinatorID)
}

// coordinatorPair checks the coordinator's task pair, agent first, stopping at
// the first status that is not ok.
func (r *taskAgentReads) coordinatorPair(ctx context.Context, workspaceID, coordinatorID string) (taskAgentOutcome, error) {
	c, err := r.coordinator(ctx, workspaceID, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return taskAgentOutcome{CoordinatorGone: true}, nil
	}
	if err != nil {
		return taskAgentOutcome{}, fmt.Errorf("read coordinator: %w", err)
	}
	out := taskAgentOutcome{Source: RunsWithSourceCoordinator, ProfileID: c.TaskAgentProfileID}
	agent, err := r.agentStatus(ctx, workspaceID, c.TaskAgentProfileID)
	if err != nil {
		return taskAgentOutcome{}, fmt.Errorf("read task agent profile: %w", err)
	}
	switch agent {
	case ProfileStatusMissing:
		out.Refusal = refusalAgentMissing
		return out, nil
	case ProfileStatusPassthrough:
		out.Refusal = refusalAgentPassthrough
		return out, nil
	}
	executor, err := r.executorStatus(ctx, c.TaskExecutorProfileID)
	if err != nil {
		return taskAgentOutcome{}, fmt.Errorf("read task executor profile: %w", err)
	}
	if executor == ProfileStatusMissing {
		out.Refusal = refusalExecutorMissing
		return out, nil
	}
	out.Meta = map[string]any{
		taskmodels.MetaKeyAgentProfileID:    c.TaskAgentProfileID,
		taskmodels.MetaKeyExecutorProfileID: c.TaskExecutorProfileID,
	}
	return out, nil
}

// runsWith computes a pending or failed create-task row's runs_with, nil when
// the row does not carry one or a read it needs failed.
func (r *taskAgentReads) runsWith(ctx context.Context, p *Proposal) *RunsWith {
	if p.Kind != "" && p.Kind != ProposalKindCreateTask {
		return nil
	}
	if p.Status != ProposalStatusPending && p.Status != ProposalStatusFailed {
		return nil
	}
	spec := p.Spec
	if p.FinalSpec != nil {
		spec = *p.FinalSpec
	}
	out, err := r.taskAgent(ctx, p.WorkspaceID, p.CoordinatorID, spec, p.StartsAgent)
	if err != nil {
		r.s.logger.Warn("runs_with read failed",
			zap.String("proposal_id", p.ID), zap.Error(err))
		return nil
	}
	switch {
	case out.StepGone, out.WorkspaceGone:
		return nil
	case out.CoordinatorGone, out.Refusal != "":
		return &RunsWith{Source: RunsWithSourceNone}
	}
	return &RunsWith{AgentProfileID: out.ProfileID, AgentProfileName: r.profileName(ctx, out.ProfileID), Source: out.Source}
}

// attachRunsWith sets RunsWith on every row that carries one, sharing reads
// across rows. It is a no-op until the decision deps are wired.
func (s *Service) attachRunsWith(ctx context.Context, rows ...*Proposal) {
	if s.decisionTasks == nil {
		return
	}
	reads := s.newTaskAgentReads()
	for _, p := range rows {
		p.RunsWith = reads.runsWith(ctx, p)
	}
}
