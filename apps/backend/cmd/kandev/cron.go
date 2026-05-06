package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
	schedulercron "github.com/kandev/kandev/internal/scheduler/cron"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// startCronScheduler builds and starts the Phase 5 shared cron loop.
// All three handlers (heartbeat, budget, routines) ride a single
// goroutine so the backend has one cron driver for all
// task-model-unification timers.
//
// Wiring is best-effort: if a collaborator is missing the matching
// handler degrades to a no-op rather than failing startup. This keeps
// development environments runnable even when, for example, no office
// service is configured.
func startCronScheduler(
	ctx context.Context,
	repos *Repositories,
	dispatcher *officeenginedispatcher.Dispatcher,
	routineSvc *officeroutines.RoutineService,
	log *logger.Logger,
) {
	heartbeat := buildHeartbeatHandler(repos, dispatcher, log)
	budget := buildBudgetHandler(repos, dispatcher, log)
	routines := schedulercron.NewRoutinesHandler(routineSvc, nil, log)
	loop := schedulercron.NewLoop(schedulercron.DefaultTickInterval, log,
		heartbeat, budget, routines)
	go loop.Start(ctx)
	log.Info("phase 5 cron loop started",
		zap.Duration("interval", schedulercron.DefaultTickInterval))
}

func buildHeartbeatHandler(
	repos *Repositories,
	dispatcher *officeenginedispatcher.Dispatcher,
	log *logger.Logger,
) *schedulercron.HeartbeatHandler {
	return schedulercron.NewHeartbeatHandler(
		&heartbeatStepLister{wf: repos.Workflow},
		&heartbeatTaskLister{tasks: repos.Task},
		&heartbeatAgentRuntime{office: repos.Office},
		dispatcher,
		nil,
		log,
	)
}

func buildBudgetHandler(
	repos *Repositories,
	dispatcher *officeenginedispatcher.Dispatcher,
	log *logger.Logger,
) *schedulercron.BudgetHandler {
	return schedulercron.NewBudgetHandler(
		&budgetEvaluator{repo: repos.Office, tasks: repos.Task},
		&budgetTaskScope{repo: repos.Office},
		dispatcher,
		nil,
		log,
	)
}

// heartbeatStepLister adapts the workflow repository to
// cron.HeartbeatStepLister. It scans every step's events JSON for the
// on_heartbeat key — Phase 6 tightens this with a SQL LIKE pre-filter
// once template authoring lands and we know the volume of steps that
// will carry the trigger. For now scanning is fine: kanban steps don't
// carry on_heartbeat so the parser quickly bails on each row.
type heartbeatStepLister struct {
	wf *workflowrepo.Repository
}

func (l *heartbeatStepLister) ListHeartbeatSteps(ctx context.Context) ([]schedulercron.HeartbeatStepInfo, error) {
	if l.wf == nil {
		return nil, nil
	}
	rows, err := l.wf.ListAllStepEventsJSON(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflow step events: %w", err)
	}
	out := make([]schedulercron.HeartbeatStepInfo, 0, len(rows))
	for _, r := range rows {
		if !schedulercron.HasHeartbeatTrigger(r.EventsJSON) {
			continue
		}
		out = append(out, schedulercron.HeartbeatStepInfo{
			StepID:         r.StepID,
			WorkflowID:     r.WorkflowID,
			CadenceSeconds: schedulercron.ParseCadenceFromEvents(r.EventsJSON),
		})
	}
	return out, nil
}

// heartbeatTaskLister adapts the task repository to
// cron.HeartbeatTaskLister.
type heartbeatTaskLister struct {
	tasks *tasksqlite.Repository
}

func (l *heartbeatTaskLister) ListActiveTasksAtStep(
	ctx context.Context, stepID string,
) ([]schedulercron.HeartbeatTaskInfo, error) {
	if l.tasks == nil {
		return nil, nil
	}
	tasks, err := l.tasks.ListTasksByWorkflowStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	out := make([]schedulercron.HeartbeatTaskInfo, 0, len(tasks))
	for _, t := range tasks {
		if t.AssigneeAgentProfileID == "" {
			continue
		}
		out = append(out, schedulercron.HeartbeatTaskInfo{
			TaskID:                 t.ID,
			WorkflowStepID:         stepID,
			AssigneeAgentProfileID: t.AssigneeAgentProfileID,
		})
	}
	return out, nil
}

// heartbeatAgentRuntime adapts the office agent + runtime repos to
// cron.HeartbeatAgentRuntime. The gate combines two checks:
//
//  1. AgentInstance.Status — paused/stopped agents never receive
//     heartbeats. (idle and working both pass; the runs scheduler's
//     own checkout layer handles concurrency.)
//  2. CooldownSec since LastRunFinishedAt — when a run finished
//     recently the gate suppresses heartbeats so the cooldown is
//     respected end-to-end (heartbeat ≠ scheduler).
type heartbeatAgentRuntime struct {
	office *officesqlite.Repository
}

func (r *heartbeatAgentRuntime) AllowFire(
	ctx context.Context, agentID string, now time.Time,
) (bool, error) {
	if r.office == nil || agentID == "" {
		return false, nil
	}
	agent, err := r.office.GetAgentInstance(ctx, agentID)
	if err != nil {
		return false, err
	}
	if agent == nil {
		return false, nil
	}
	switch agent.Status {
	case officemodels.AgentStatusPaused, officemodels.AgentStatusStopped:
		return false, nil
	}
	if agent.LastRunFinishedAt != nil && agent.CooldownSec > 0 {
		gateUntil := agent.LastRunFinishedAt.Add(time.Duration(agent.CooldownSec) * time.Second)
		if now.Before(gateUntil) {
			return false, nil
		}
	}
	return true, nil
}

// budgetEvaluator adapts the office repository to
// cron.BudgetEvaluator. It walks every workspace, lists policies, and
// computes spend per policy using the same projections the office
// CostService uses for pre-execution checks.
type budgetEvaluator struct {
	repo  *officesqlite.Repository
	tasks *tasksqlite.Repository
}

func (e *budgetEvaluator) EvaluatePolicies(ctx context.Context) ([]schedulercron.BudgetCheckResult, error) {
	if e.repo == nil || e.tasks == nil {
		return nil, nil
	}
	workspaces, err := e.tasks.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	var out []schedulercron.BudgetCheckResult
	for _, ws := range workspaces {
		policies, err := e.repo.ListBudgetPolicies(ctx, ws.ID)
		if err != nil {
			return nil, fmt.Errorf("list budget policies (%s): %w", ws.ID, err)
		}
		for _, p := range policies {
			spent, err := e.spendForPolicy(ctx, p, ws.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, schedulercron.BudgetCheckResult{
				WorkspaceID:   ws.ID,
				ScopeType:     p.ScopeType,
				ScopeID:       p.ScopeID,
				SpentSubcents: spent,
				LimitSubcents: p.LimitSubcents,
				Period:        p.Period,
			})
		}
	}
	return out, nil
}

// spendForPolicy applies the period filter ("monthly" resets at the 1st
// UTC; "total" or anything else returns the lifetime sum).
func (e *budgetEvaluator) spendForPolicy(
	ctx context.Context, p *officemodels.BudgetPolicy, workspaceID string,
) (int64, error) {
	var since time.Time
	if p.Period == "monthly" {
		now := time.Now().UTC()
		since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	switch p.ScopeType {
	case "agent":
		return e.repo.GetCostForAgentSince(ctx, p.ScopeID, since)
	case "project":
		return e.repo.GetCostForProjectSince(ctx, p.ScopeID, since)
	case "workspace":
		return e.repo.SumCostsSince(ctx, workspaceID, since)
	}
	return 0, nil
}

// budgetTaskScope adapts the office repository to
// cron.BudgetTaskScope. It returns the workspace's coordination task
// for every scope today. Phase 6 lands the coordination task and a
// dedicated lookup by role; until then the resolver returns empty
// for workspaces without one configured, which the budget handler
// interprets as "no fire, mark dedup so we don't churn".
type budgetTaskScope struct {
	repo *officesqlite.Repository
}

func (s *budgetTaskScope) ResolveAlertTaskID(
	_ context.Context, _, _, _ string,
) (string, error) {
	// TODO(phase 6): look up the workspace's coordination task by the
	// agent role configured on it (CEO / standing). For now the
	// handler returns no task — the budget cron is quiet by design
	// until Phase 6 lands the on_budget_alert wiring on the
	// coordination workflow.
	return "", nil
}
