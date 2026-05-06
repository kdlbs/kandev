package costs

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Repository abstracts the persistence layer for costs and budgets.
type Repository interface {
	CreateCostEvent(ctx context.Context, event *CostEvent) error
	ListCostEvents(ctx context.Context, workspaceID string) ([]*CostEvent, error)
	GetCostsByAgent(ctx context.Context, workspaceID string) ([]*CostBreakdown, error)
	GetCostsByProject(ctx context.Context, workspaceID string) ([]*CostBreakdown, error)
	GetCostsByModel(ctx context.Context, workspaceID string) ([]*CostBreakdown, error)
	GetCostsByProvider(ctx context.Context, workspaceID string) ([]*CostBreakdown, error)
	SumCosts(ctx context.Context, workspaceID string) (int64, error)
	SumCostsSince(ctx context.Context, workspaceID string, since time.Time) (int64, error)
	GetCostForAgent(ctx context.Context, agentID string) (int64, error)
	GetCostForAgentSince(ctx context.Context, agentID string, since time.Time) (int64, error)
	GetCostForProject(ctx context.Context, projectID string) (int64, error)
	GetCostForProjectSince(ctx context.Context, projectID string, since time.Time) (int64, error)
	CreateBudgetPolicy(ctx context.Context, policy *BudgetPolicy) error
	GetBudgetPolicy(ctx context.Context, id string) (*BudgetPolicy, error)
	ListBudgetPolicies(ctx context.Context, workspaceID string) ([]*BudgetPolicy, error)
	UpdateBudgetPolicy(ctx context.Context, policy *BudgetPolicy) error
	DeleteBudgetPolicy(ctx context.Context, id string) error
	UpdateAgentStatusFields(ctx context.Context, agentID, status, pauseReason string) error
}

// CostService handles cost recording, summaries, and budget evaluation.
type CostService struct {
	repo     Repository
	logger   *logger.Logger
	activity shared.ActivityLogger
	agents   shared.AgentReader
	agentW   shared.AgentWriter
}

// NewCostService creates a new CostService.
func NewCostService(
	repo Repository,
	log *logger.Logger,
	activity shared.ActivityLogger,
	agents shared.AgentReader,
	agentW shared.AgentWriter,
) *CostService {
	return &CostService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "costs-service")),
		activity: activity,
		agents:   agents,
		agentW:   agentW,
	}
}

// RecordCostEvent stores a cost event with caller-provided cost subcents.
// Cost computation now lives in the subscriber (Layer A / B lookup); this
// helper is the manual-entry / test-harness path that records a row
// verbatim. Token counts are stored as int64.
func (s *CostService) RecordCostEvent(
	ctx context.Context,
	sessionID, taskID, agentInstanceID, projectID string,
	model, provider string,
	tokensIn, tokensCachedIn, tokensOut, costSubcents int64,
	estimated bool,
) (*CostEvent, error) {
	event := &CostEvent{
		SessionID:      sessionID,
		TaskID:         taskID,
		AgentProfileID: agentInstanceID,
		ProjectID:      projectID,
		Model:          model,
		Provider:       provider,
		TokensIn:       tokensIn,
		TokensCachedIn: tokensCachedIn,
		TokensOut:      tokensOut,
		CostSubcents:   costSubcents,
		Estimated:      estimated,
		OccurredAt:     time.Now().UTC(),
	}

	if err := s.repo.CreateCostEvent(ctx, event); err != nil {
		return nil, err
	}

	s.logger.Info("cost event recorded",
		zap.String("session_id", sessionID),
		zap.String("model", model),
		zap.Int64("cost_subcents", costSubcents),
		zap.Bool("estimated", estimated))

	return event, nil
}

// GetCostSummary returns total spend in subcents for a workspace.
// Implements shared.CostChecker.
func (s *CostService) GetCostSummary(ctx context.Context, wsID string) (int64, error) {
	return s.repo.SumCosts(ctx, wsID)
}

// ListCostEvents returns cost events for a workspace.
func (s *CostService) ListCostEvents(ctx context.Context, wsID string) ([]*CostEvent, error) {
	return s.repo.ListCostEvents(ctx, wsID)
}

// GetCostsByAgent returns costs grouped by agent.
func (s *CostService) GetCostsByAgent(ctx context.Context, wsID string) ([]*CostBreakdown, error) {
	return s.repo.GetCostsByAgent(ctx, wsID)
}

// GetCostsByProject returns costs grouped by project.
func (s *CostService) GetCostsByProject(ctx context.Context, wsID string) ([]*CostBreakdown, error) {
	return s.repo.GetCostsByProject(ctx, wsID)
}

// GetCostsByModel returns costs grouped by model.
func (s *CostService) GetCostsByModel(ctx context.Context, wsID string) ([]*CostBreakdown, error) {
	return s.repo.GetCostsByModel(ctx, wsID)
}

// GetCostsByProvider returns costs grouped by provider (Claude / OpenAI / Gemini).
func (s *CostService) GetCostsByProvider(ctx context.Context, wsID string) ([]*CostBreakdown, error) {
	return s.repo.GetCostsByProvider(ctx, wsID)
}

// GetCostsBreakdown bundles the four cost views surfaced on the costs
// overview page (total + by-agent / by-project / by-model). All four
// aggregations run concurrently via errgroup so total wall time tracks the
// slowest query rather than the sum (Stream D of office optimization).
//
// Each underlying repo call uses its own DB connection from the shared
// read-only pool. SQLite WAL mode gives concurrent readers a consistent
// snapshot of the cost_events table at query time, so the four views are
// drift-free for the typical sub-second query latency without an explicit
// BEGIN/COMMIT block.
func (s *CostService) GetCostsBreakdown(
	ctx context.Context, wsID string,
) (total int64, byAgent, byProject, byModel, byProvider []*CostBreakdown, err error) {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		t, e := s.repo.SumCosts(gctx, wsID)
		if e != nil {
			return e
		}
		total = t
		return nil
	})
	g.Go(func() error {
		v, e := s.repo.GetCostsByAgent(gctx, wsID)
		if e != nil {
			return e
		}
		byAgent = v
		return nil
	})
	g.Go(func() error {
		v, e := s.repo.GetCostsByProject(gctx, wsID)
		if e != nil {
			return e
		}
		byProject = v
		return nil
	})
	g.Go(func() error {
		v, e := s.repo.GetCostsByModel(gctx, wsID)
		if e != nil {
			return e
		}
		byModel = v
		return nil
	})
	g.Go(func() error {
		v, e := s.repo.GetCostsByProvider(gctx, wsID)
		if e != nil {
			return e
		}
		byProvider = v
		return nil
	})
	if err = g.Wait(); err != nil {
		return 0, nil, nil, nil, nil, err
	}
	if byAgent == nil {
		byAgent = []*CostBreakdown{}
	}
	if byProject == nil {
		byProject = []*CostBreakdown{}
	}
	if byModel == nil {
		byModel = []*CostBreakdown{}
	}
	if byProvider == nil {
		byProvider = []*CostBreakdown{}
	}
	return total, byAgent, byProject, byModel, byProvider, nil
}
