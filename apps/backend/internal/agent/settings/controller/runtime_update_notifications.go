package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"net/url"
	"strings"
)

type RuntimeUpdateNotifier interface {
	HandleAgentRuntimeUpdate(context.Context, agents.RuntimeUpdateNotice)
	HandleAgentRuntimeUpdates(context.Context, []agents.RuntimeUpdateNotice)
}

type runtimeAvailabilityObservation struct {
	agentID    string
	runtimeID  string
	observedAt time.Time
	notice     *agents.RuntimeUpdateNotice
}

type runtimeAvailabilityCollector struct {
	mu     sync.Mutex
	latest map[string]runtimeAvailabilityObservation
	wake   chan struct{}
	closed bool
}

type runtimeUpdateDeliveryQueue struct {
	mu     sync.Mutex
	latest map[string]agents.RuntimeUpdateNotice
	wake   chan struct{}
	closed bool
}

type runtimeAvailabilityBatch struct {
	deliveries *runtimeUpdateDeliveryQueue
	ctx        context.Context
	pending    map[string]agents.RuntimeUpdateNotice
	timer      *time.Timer
	timerC     <-chan time.Time
	deadline   time.Time
}

func newRuntimeAvailabilityCollector() *runtimeAvailabilityCollector {
	return &runtimeAvailabilityCollector{
		latest: make(map[string]runtimeAvailabilityObservation),
		wake:   make(chan struct{}, 1),
	}
}

func (c *runtimeAvailabilityCollector) admit(ctx, workerCtx context.Context, observation runtimeAvailabilityObservation) bool {
	if ctx.Err() != nil || workerCtx.Err() != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || ctx.Err() != nil || workerCtx.Err() != nil {
		return false
	}
	observation.observedAt = time.Now()
	key := observation.agentID + "\x00" + observation.runtimeID
	c.latest[key] = observation
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return true
}

func (c *runtimeAvailabilityCollector) takeLatest() []runtimeAvailabilityObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	observations := make([]runtimeAvailabilityObservation, 0, len(c.latest))
	for _, observation := range c.latest {
		observations = append(observations, observation)
	}
	clear(c.latest)
	return observations
}

func (c *runtimeAvailabilityCollector) close() {
	c.mu.Lock()
	c.closed = true
	clear(c.latest)
	c.mu.Unlock()
}

func newRuntimeUpdateDeliveryQueue() *runtimeUpdateDeliveryQueue {
	return &runtimeUpdateDeliveryQueue{
		latest: make(map[string]agents.RuntimeUpdateNotice),
		wake:   make(chan struct{}, 1),
	}
}

func (q *runtimeUpdateDeliveryQueue) admit(ctx context.Context, notices []agents.RuntimeUpdateNotice) bool {
	if len(notices) == 0 || ctx.Err() != nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || ctx.Err() != nil {
		return false
	}
	for _, notice := range notices {
		key := notice.AgentID + "\x00" + notice.RuntimeID
		q.latest[key] = notice
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return true
}

func (q *runtimeUpdateDeliveryQueue) takeLatest() []agents.RuntimeUpdateNotice {
	q.mu.Lock()
	defer q.mu.Unlock()
	notices := make([]agents.RuntimeUpdateNotice, 0, len(q.latest))
	for _, notice := range q.latest {
		notices = append(notices, notice)
	}
	clear(q.latest)
	return notices
}

func (q *runtimeUpdateDeliveryQueue) close() {
	q.mu.Lock()
	q.closed = true
	clear(q.latest)
	q.mu.Unlock()
}

func (c *Controller) SetRuntimeUpdateNotifier(notifier RuntimeUpdateNotifier) {
	c.runtimeUpdateNotifier = notifier
}

func (c *Controller) publishRuntimeStatus(ctx context.Context, status dto.AgentUpdateStatusDTO) {
	notice := agents.RuntimeUpdateNotice{AgentID: status.AgentName, RuntimeID: status.RuntimeID, DisplayName: status.DisplayName, URL: "/settings/agents#runtime-update-" + url.PathEscape(status.AgentName)}
	available := status.Available && status.Enabled && status.CheckState == dto.AgentUpdateCheckStateUpdateAvailable && c.trustedRuntimeUpdateIdentity(status.AgentName, status.RuntimeID)
	var availabilityNotice *agents.RuntimeUpdateNotice
	if available {
		notice.PreviousVersion, notice.Version, notice.Status = status.EffectiveVersion, status.LatestVersion, "available"
		notice.OccurrenceID = runtimeNoticeKey(status.AgentName, status.RuntimeID, status.LatestVersion, "available")
		copy := notice
		availabilityNotice = &copy
	}
	if c.runtimeUpdateNotifier != nil {
		if outcome := status.LastOutcome; outcome != nil && (outcome.Status == managedruntime.UpdateOutcomeSucceeded || outcome.Status == managedruntime.UpdateOutcomeFailed || outcome.Status == managedruntime.UpdateOutcomeInterrupted) {
			notice.PreviousVersion, notice.Version, notice.Status = outcome.PreviousVersion, outcome.TargetVersion, outcome.Status
			notice.OccurrenceID = runtimeNoticeKey(status.AgentName, status.RuntimeID, outcome.TargetVersion, outcome.Status, outcome.ID)
			c.runtimeUpdateNotifier.HandleAgentRuntimeUpdate(ctx, notice)
		}
	}
	if available {
		c.observeRuntimeAvailability(ctx, status.AgentName, status.RuntimeID, availabilityNotice)
	} else {
		c.observeRuntimeAvailability(ctx, status.AgentName, status.RuntimeID, nil)
	}
}

func (c *Controller) trustedRuntimeUpdateIdentity(agentID, runtimeID string) bool {
	agent, found := c.agentRegistry.Get(agentID)
	if !found || runtimeID == "" {
		return false
	}
	return c.runtimeUpdateCapabilities(agent).RuntimeID == runtimeID
}

func (c *Controller) observeRuntimeAvailability(ctx context.Context, agentID, runtimeID string, notice *agents.RuntimeUpdateNotice) {
	if agentID == "" || runtimeID == "" || !c.trustedRuntimeUpdateIdentity(agentID, runtimeID) {
		return
	}
	var stableNotice *agents.RuntimeUpdateNotice
	if notice != nil {
		copy := *notice
		stableNotice = &copy
	}
	c.runtimeBackgroundMu.Lock()
	worker := c.runtimeBackground
	c.runtimeBackgroundMu.Unlock()
	if worker == nil || worker.collector == nil {
		return
	}
	worker.collector.admit(ctx, worker.ctx, runtimeAvailabilityObservation{
		agentID: agentID, runtimeID: runtimeID, notice: stableNotice,
	})
}

func (c *Controller) runRuntimeAvailabilityCollector(
	ctx context.Context,
	collector *runtimeAvailabilityCollector,
	deliveries *runtimeUpdateDeliveryQueue,
) {
	defer collector.close()
	batch := runtimeAvailabilityBatch{
		deliveries: deliveries,
		ctx:        ctx,
		pending:    make(map[string]agents.RuntimeUpdateNotice),
	}
	defer batch.stopTimer()
	for {
		select {
		case <-ctx.Done():
			return
		case <-collector.wake:
			batch.applyObservations(collector.takeLatest())
		case <-batch.timerC:
			batch.applyObservations(collector.takeLatest())
			if batch.expired() {
				batch.flush()
			}
		}
	}
}

func (b *runtimeAvailabilityBatch) stopTimer() {
	if b.timer == nil {
		return
	}
	if !b.timer.Stop() {
		select {
		case <-b.timer.C:
		default:
		}
	}
	b.timer = nil
	b.timerC = nil
	b.deadline = time.Time{}
}

func (b *runtimeAvailabilityBatch) flush() {
	notices := make([]agents.RuntimeUpdateNotice, 0, len(b.pending))
	for _, notice := range b.pending {
		notices = append(notices, notice)
	}
	clear(b.pending)
	b.stopTimer()
	b.deliveries.admit(b.ctx, notices)
}

func (b *runtimeAvailabilityBatch) applyObservations(observations []runtimeAvailabilityObservation) {
	sort.Slice(observations, func(i, j int) bool {
		return runtimeAvailabilityObservationBefore(observations[i], observations[j])
	})
	for _, observation := range observations {
		b.applyObservation(observation)
	}
}

func runtimeAvailabilityObservationBefore(a, b runtimeAvailabilityObservation) bool {
	if a.observedAt.Equal(b.observedAt) {
		return a.agentID+"\x00"+a.runtimeID < b.agentID+"\x00"+b.runtimeID
	}
	return a.observedAt.Before(b.observedAt)
}

func (b *runtimeAvailabilityBatch) applyObservation(observation runtimeAvailabilityObservation) {
	if !b.deadline.IsZero() && !observation.observedAt.Before(b.deadline) {
		b.flush()
	}
	key := observation.agentID + "\x00" + observation.runtimeID
	if observation.notice == nil {
		delete(b.pending, key)
		return
	}
	if b.deadline.IsZero() {
		b.startTimer(observation.observedAt)
	}
	b.pending[key] = *observation.notice
}

func (c *Controller) runRuntimeUpdateDeliveryWorker(ctx context.Context, deliveries *runtimeUpdateDeliveryQueue) {
	defer deliveries.close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deliveries.wake:
			notices := deliveries.takeLatest()
			c.revalidateAndDeliverRuntimeUpdateSummary(ctx, notices)
		}
	}
}

func (b *runtimeAvailabilityBatch) startTimer(observedAt time.Time) {
	b.deadline = observedAt.Add(runtimeUpdateAvailabilityWindow)
	delay := time.Until(b.deadline)
	if delay < 0 {
		delay = 0
	}
	b.timer = time.NewTimer(delay)
	b.timerC = b.timer.C
}

func (b *runtimeAvailabilityBatch) expired() bool {
	return !b.deadline.IsZero() && !time.Now().Before(b.deadline)
}

func (c *Controller) revalidateAndDeliverRuntimeUpdateSummary(ctx context.Context, notices []agents.RuntimeUpdateNotice) {
	if len(notices) == 0 || ctx.Err() != nil {
		return
	}
	response, err := c.ListAgentUpdateStatuses(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.logger.Debug("runtime availability summary revalidation unavailable")
		}
		return
	}
	current := make(map[string]dto.AgentUpdateStatusDTO, len(response.Statuses))
	for _, status := range response.Statuses {
		current[status.AgentName+"\x00"+status.RuntimeID] = status
	}
	eligible := make([]agents.RuntimeUpdateNotice, 0, len(notices))
	for _, notice := range notices {
		status, found := current[notice.AgentID+"\x00"+notice.RuntimeID]
		if !found {
			continue
		}
		refreshed, isEligible := c.refreshRuntimeAvailabilityNotice(notice, status)
		if isEligible {
			notice = refreshed
			eligible = append(eligible, notice)
		}
	}
	if len(eligible) == 0 || ctx.Err() != nil || c.runtimeUpdateNotifier == nil {
		return
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].AgentID != eligible[j].AgentID {
			return eligible[i].AgentID < eligible[j].AgentID
		}
		return eligible[i].RuntimeID < eligible[j].RuntimeID
	})
	c.runtimeUpdateNotifier.HandleAgentRuntimeUpdates(ctx, eligible)
}

func (c *Controller) refreshRuntimeAvailabilityNotice(
	notice agents.RuntimeUpdateNotice,
	status dto.AgentUpdateStatusDTO,
) (agents.RuntimeUpdateNotice, bool) {
	if !status.Available || !status.Enabled || status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable || status.LatestVersion != notice.Version {
		return notice, false
	}
	if !c.trustedRuntimeUpdateIdentity(status.AgentName, status.RuntimeID) {
		return notice, false
	}
	notice.AgentID = status.AgentName
	notice.RuntimeID = status.RuntimeID
	notice.DisplayName = status.DisplayName
	notice.PreviousVersion = status.EffectiveVersion
	notice.Version = status.LatestVersion
	notice.URL = "/settings/agents#runtime-update-" + url.PathEscape(status.AgentName)
	return notice, true
}

func runtimeNoticeKey(parts ...string) string {
	return fmt.Sprintf("agent-runtime:%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

func (c *Controller) ReplayRuntimeUpdateNotices(ctx context.Context) error {
	response, err := c.ListAgentUpdateStatuses(ctx)
	if err != nil {
		return err
	}
	for _, status := range response.Statuses {
		c.publishRuntimeStatus(ctx, status)
	}
	return nil
}
