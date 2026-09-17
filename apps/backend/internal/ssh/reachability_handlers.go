package ssh

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	reachabilitypkg "github.com/kandev/kandev/internal/executors/reachability"
	"github.com/kandev/kandev/internal/task/models"
)

// reachabilityConfigValidator is the production seam over the runtime's SSH
// reachability prober, used only to check whether an executor's config
// resolves to a dialable target — no probe is actually run here.
var reachabilityConfigValidator = agentruntime.NewSSHReachabilityProber()

// ReachabilityLister is the narrow repository slice the reachability GET
// routes need: every stored record in one query (the list route) or a
// single record (the single-executor route). Executor lookup itself reuses
// ExecutorFetcher, already wired on Handler for the existing SSH routes.
type ReachabilityLister interface {
	ListExecutors(ctx context.Context) ([]*models.Executor, error)
	ListExecutorReachability(ctx context.Context) ([]*models.ExecutorReachability, error)
	GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error)
}

// ReachabilityProber is the narrow slice of *reachability.Poller the
// immediate-probe route needs. Declared here (not imported as the concrete
// type) so a handler test can substitute a fake without spinning up a real
// poller when it only needs to exercise coalescing or 404/400/409
// short-circuits.
type ReachabilityProber interface {
	ProbeAndWait(executor *models.Executor) (reachabilitypkg.ProbeResult, bool)
	EffectiveIntervalSeconds() int
}

// --- HTTP handlers ---

func (h *Handler) httpListReachability(c *gin.Context) {
	dtos, status, err := h.listReachability(c.Request.Context())
	if err != nil {
		c.JSON(status, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(status, dtos)
}

func (h *Handler) httpGetReachability(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: errExecutorIDRequired})
		return
	}
	dto, status, err := h.getReachability(c.Request.Context(), id)
	if err != nil {
		c.JSON(status, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(status, dto)
}

func (h *Handler) httpProbeReachability(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: errExecutorIDRequired})
		return
	}
	dto, status, err := h.probeReachability(c.Request.Context(), id)
	if err != nil {
		c.JSON(status, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(status, dto)
}

// --- Testable cores ---

// listReachability returns one DTO per SSH executor that is not
// soft-deleted, ordered by executor_id ascending — the same total order the
// poller's own pass uses. An executor whose status is not active is still
// included, carrying its retained record.
func (h *Handler) listReachability(ctx context.Context) ([]reachabilitypkg.RecordDTO, int, error) {
	if h.reachabilityRepo == nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("reachability listing not wired")
	}
	executors, err := h.reachabilityRepo.ListExecutors(ctx)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("list executors: %w", err)
	}
	records, err := h.reachabilityRepo.ListExecutorReachability(ctx)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("list executor reachability: %w", err)
	}
	byExecutor := make(map[string]*models.ExecutorReachability, len(records))
	for _, record := range records {
		byExecutor[record.ExecutorID] = record
	}

	sshExecutors := filterSSHExecutors(executors)
	sort.Slice(sshExecutors, func(i, j int) bool { return sshExecutors[i].ID < sshExecutors[j].ID })

	interval := h.reachabilityIntervalSeconds()
	dtos := make([]reachabilitypkg.RecordDTO, 0, len(sshExecutors))
	for _, executor := range sshExecutors {
		dtos = append(dtos, buildReachabilityDTOFromRecord(executor, byExecutor[executor.ID], interval))
	}
	return dtos, http.StatusOK, nil
}

// getReachability returns the single record for executorID. Never 404s for
// a missing record — only for a missing or soft-deleted executor — per
// AC-EXECUTORS-SSH-REACHABILITY-002.5.
func (h *Handler) getReachability(ctx context.Context, executorID string) (*reachabilitypkg.RecordDTO, int, error) {
	executor, status, err := h.loadSSHExecutor(ctx, executorID)
	if err != nil {
		return nil, status, err
	}
	if h.reachabilityRepo == nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("reachability listing not wired")
	}
	record, err := h.reachabilityRepo.GetExecutorReachability(ctx, executorID)
	if err != nil && !errors.Is(err, models.ErrExecutorReachabilityNotFound) {
		return nil, http.StatusInternalServerError, fmt.Errorf("get executor reachability: %w", err)
	}
	if errors.Is(err, models.ErrExecutorReachabilityNotFound) {
		record = nil
	}
	dto := buildReachabilityDTOFromRecord(executor, record, h.reachabilityIntervalSeconds())
	return &dto, http.StatusOK, nil
}

// probeReachability runs (or joins an in-flight) synchronous probe for
// executorID and returns its persisted record. Coalesces concurrent callers
// through reachabilityPoller.ProbeAndWait's own singleflight group.
func (h *Handler) probeReachability(ctx context.Context, executorID string) (*reachabilitypkg.RecordDTO, int, error) {
	executor, status, err := h.loadSSHExecutor(ctx, executorID)
	if err != nil {
		return nil, status, err
	}
	if executor.Status != models.ExecutorStatusActive {
		return nil, http.StatusConflict, fmt.Errorf("executor %q is not active", executorID)
	}
	if h.reachabilityPoller == nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("reachability probing not wired")
	}
	result, ok := h.reachabilityPoller.ProbeAndWait(executor)
	if !ok {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("reachability poller is not accepting probes")
	}
	dto := reachabilitypkg.BuildRecordDTO(executorID, result.Record, h.reachabilityPoller.EffectiveIntervalSeconds(), result.Persisted)
	return &dto, http.StatusOK, nil
}

// loadSSHExecutor enforces the 404/400 split the reachability routes need:
// 404 for a nonexistent or soft-deleted id, 400 only when the executor
// exists but isn't type ssh. Deliberately does not attempt config
// resolution (unlike resolveSSHTarget) — an unresolvable ssh config is a 200
// with reason config, decided by the caller, never a 400 here.
func (h *Handler) loadSSHExecutor(ctx context.Context, executorID string) (*models.Executor, int, error) {
	executor, err := h.executorFetcher.GetExecutor(ctx, executorID)
	if err != nil {
		if errors.Is(err, models.ErrExecutorNotFound) {
			return nil, http.StatusNotFound, fmt.Errorf("executor %q not found: %w", executorID, err)
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("look up executor %q: %w", executorID, err)
	}
	if executor.Type != models.ExecutorTypeSSH {
		return nil, http.StatusBadRequest, fmt.Errorf("executor %q is not an SSH executor", executorID)
	}
	return executor, http.StatusOK, nil
}

func (h *Handler) reachabilityIntervalSeconds() int {
	if h.reachabilityPoller == nil {
		return 0
	}
	return h.reachabilityPoller.EffectiveIntervalSeconds()
}

func filterSSHExecutors(executors []*models.Executor) []*models.Executor {
	out := make([]*models.Executor, 0, len(executors))
	for _, executor := range executors {
		if executor != nil && executor.Type == models.ExecutorTypeSSH {
			out = append(out, executor)
		}
	}
	return out
}

// buildReachabilityDTOFromRecord projects executor/record into the wire
// shape without any I/O. record nil means "no record yet" — that synthesizes
// the unknown placeholder unless the executor's own config can't resolve a
// target, in which case AC-EXECUTORS-SSH-REACHABILITY-002.5's sibling rule
// applies: 200 with reason config, the same outcome a real probe would reach
// on its very first attempt, rather than a misleading "not probed yet".
func buildReachabilityDTOFromRecord(executor *models.Executor, record *models.ExecutorReachability, intervalSeconds int) reachabilitypkg.RecordDTO {
	if record != nil {
		return reachabilitypkg.BuildRecordDTO(executor.ID, record, intervalSeconds, true)
	}
	if err := reachabilityConfigValidator.ValidateConfig(executor.Config); err != nil {
		synthetic := &models.ExecutorReachability{
			ExecutorID: executor.ID,
			State:      models.ExecutorReachabilityStateUnreachable,
			Reason:     models.ExecutorReachabilityReasonConfig,
			Message:    err.Error(),
			Host:       executor.Config["ssh_host"],
		}
		return reachabilitypkg.BuildRecordDTO(executor.ID, synthetic, intervalSeconds, false)
	}
	return reachabilitypkg.BuildRecordDTO(executor.ID, nil, intervalSeconds, false)
}
