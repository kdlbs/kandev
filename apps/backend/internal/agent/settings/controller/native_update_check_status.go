package controller

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// nativeCheckLifetime bounds native check executions. Executions register
// while it is open; close cancels them and waits until every registered
// execution has returned. A closed lifetime is never reopened.
type nativeCheckLifetime struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	active int
	wg     sync.WaitGroup
}

func newNativeCheckLifetime() *nativeCheckLifetime {
	ctx, cancel := context.WithCancel(context.Background())
	return &nativeCheckLifetime{ctx: ctx, cancel: cancel}
}

// admit registers one execution, or reports false once the lifetime closed.
func (l *nativeCheckLifetime) admit() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.active++
	l.wg.Add(1)
	return true
}

func (l *nativeCheckLifetime) done() {
	l.mu.Lock()
	l.active--
	l.mu.Unlock()
	l.wg.Done()
}

func (l *nativeCheckLifetime) inFlight() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

func (l *nativeCheckLifetime) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *nativeCheckLifetime) close() {
	l.mu.Lock()
	l.closed = true
	l.cancel()
	l.mu.Unlock()
	l.wg.Wait()
}

func (c *Controller) currentNativeCheckLifetime() *nativeCheckLifetime {
	c.runtimeBackgroundMu.Lock()
	defer c.runtimeBackgroundMu.Unlock()
	if c.nativeCheckLifetime == nil {
		c.nativeCheckLifetime = newNativeCheckLifetime()
	}
	return c.nativeCheckLifetime
}

// renewNativeCheckLifetimeLocked installs a fresh lifetime after a stop.
// The caller holds runtimeBackgroundMu.
func (c *Controller) renewNativeCheckLifetimeLocked() {
	if c.nativeCheckLifetime == nil || c.nativeCheckLifetime.isClosed() {
		c.nativeCheckLifetime = newNativeCheckLifetime()
	}
}

// closeNativeCheckLifetime cancels admitted native checks and waits for them.
func (c *Controller) closeNativeCheckLifetime() {
	lifetime := c.currentNativeCheckLifetime()
	if pending := lifetime.inFlight(); pending > 0 && c.logger != nil {
		c.logger.Debug("cancelling native update checks", zap.Int("pending", pending))
	}
	lifetime.close()
}

// SetNativeUpdateCheckRunner injects the native check executor and clears
// prior status results. Passing nil restores the process runner.
func (c *Controller) SetNativeUpdateCheckRunner(runner nativeUpdateCheckRunner) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	c.nativeUpdateCheckRunner = runner
	c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
}

func (c *Controller) nativeRunner() nativeUpdateCheckRunner {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if c.nativeUpdateCheckRunner == nil {
		return execNativeUpdateCheckRunner{}
	}
	return c.nativeUpdateCheckRunner
}

func (c *Controller) runtimeUpdateStatusSemaphore() chan struct{} {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if c.runtimeUpdateStatusLookup == nil {
		c.runtimeUpdateStatusLookup = make(chan struct{}, runtimeUpdateStatusMaxConcurrent)
	}
	return c.runtimeUpdateStatusLookup
}

// InvalidateNativeUpdateCheck discards an agent's native results. Checks that
// started earlier can still answer their waiting subscribers but never fill
// the cache for later requests.
func (c *Controller) InvalidateNativeUpdateCheck(agentName string) {
	if agentName == "" {
		return
	}
	prefix := nativeUpdateCheckKeyPrefix(agentName)
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if c.nativeUpdateCheckGeneration == nil {
		c.nativeUpdateCheckGeneration = make(map[string]uint64)
	}
	c.nativeUpdateCheckGeneration[agentName]++
	for key := range c.runtimeUpdateStatusCache {
		if strings.HasPrefix(key, prefix) {
			delete(c.runtimeUpdateStatusCache, key)
		}
	}
}

// invalidateRuntimeUpdateStatusForJob receives terminal job invalidations.
func (c *Controller) invalidateRuntimeUpdateStatusForJob(invalidation RuntimeStatusInvalidation) {
	if invalidation.Package != "" {
		c.InvalidateRuntimeUpdateStatus(invalidation.Package)
	}
	c.InvalidateNativeUpdateCheck(invalidation.AgentName)
}

func nativeUpdateCheckSpec(ag agents.Agent) (agents.NativeUpdateCheckSpec, bool) {
	checker, ok := ag.(agents.NativeUpdateCheckAgent)
	if !ok {
		return agents.NativeUpdateCheckSpec{}, false
	}
	spec := checker.NativeUpdateCheck()
	return spec, !spec.Command.IsEmpty() && spec.Parse != nil
}

// nativeUpdateCheckEligible is the single guard for every native entry point:
// disabled agents and agents that discovery does not report never resolve,
// fingerprint, or run a check.
func (c *Controller) nativeUpdateCheckEligible(ag agents.Agent, available map[string]bool) bool {
	return ag.Enabled() && (c.discovery == nil || available[ag.ID()])
}

type nativeUpdateCheckLookup struct {
	agentName      string
	currentVersion string
	executable     string
	key            string
	generation     uint64
	spec           agents.NativeUpdateCheckSpec
}

// prepareNativeUpdateCheck computes the cache identity without starting a
// process: the resolved executable, runtime version, generation, and update
// configuration fingerprint.
func (c *Controller) prepareNativeUpdateCheck(agentName, currentVersion string, spec agents.NativeUpdateCheckSpec) nativeUpdateCheckLookup {
	runner := c.nativeRunner()
	executable, err := runner.Resolve(spec.Command)
	if err != nil {
		executable = ""
	}
	workingDir, _ := os.Getwd()
	fingerprint := spec.Context.Fingerprint(agents.NewNativeUpdateCheckResolver(runner.Environment(), workingDir, executable))
	c.runtimeUpdateStatusMu.Lock()
	generation := c.nativeUpdateCheckGeneration[agentName]
	c.runtimeUpdateStatusMu.Unlock()
	return nativeUpdateCheckLookup{
		agentName: agentName, currentVersion: currentVersion, executable: executable,
		generation: generation, spec: spec,
		key: nativeUpdateCheckCacheKey(agentName, generation, spec.Command.Args(), executable, currentVersion, fingerprint),
	}
}

func nativeUpdateCheckKeyPrefix(agentName string) string {
	return "native\x00" + agentName + "\x00"
}

func nativeUpdateCheckCacheKey(agentName string, generation uint64, argv []string, executable, version, fingerprint string) string {
	parts := make([]string, 0, len(argv)+4)
	parts = append(parts, strconv.FormatUint(generation, 10), strconv.Itoa(len(argv)))
	parts = append(parts, argv...)
	parts = append(parts, executable, version, fingerprint)
	return nativeUpdateCheckKeyPrefix(agentName) + strings.Join(parts, "\x00")
}

// nativeUpdateCheckEntry returns a cached result or joins the single shared
// execution for the lookup key.
func (c *Controller) nativeUpdateCheckEntry(ctx context.Context, lookup nativeUpdateCheckLookup, now time.Time) runtimeUpdateStatusCacheEntry {
	if ctx.Err() != nil {
		return runtimeUpdateStatusCacheEntry{}
	}
	if entry, ok := c.freshRuntimeUpdateStatusEntry(lookup.key, now); ok {
		return entry
	}
	shared := context.WithoutCancel(ctx)
	result := c.runtimeUpdateStatusFlight.DoChan(lookup.key, func() (interface{}, error) {
		return c.executeNativeUpdateCheck(shared, lookup, now), nil
	})
	select {
	case <-ctx.Done():
		return runtimeUpdateStatusCacheEntry{}
	case value := <-result:
		return value.Val.(runtimeUpdateStatusCacheEntry)
	}
}

func (c *Controller) freshRuntimeUpdateStatusEntry(key string, now time.Time) (runtimeUpdateStatusCacheEntry, bool) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	entry, ok := c.runtimeUpdateStatusCache[key]
	return entry, ok && now.Before(entry.expiresAt)
}

func (c *Controller) executeNativeUpdateCheck(ctx context.Context, lookup nativeUpdateCheckLookup, now time.Time) runtimeUpdateStatusCacheEntry {
	lifetime := c.currentNativeCheckLifetime()
	if !lifetime.admit() {
		return nativeCancelledEntry()
	}
	defer lifetime.done()
	slots := c.runtimeUpdateStatusSemaphore()
	select {
	case slots <- struct{}{}:
	case <-lifetime.ctx.Done():
		return nativeCancelledEntry()
	}
	defer func() { <-slots }()
	if entry, ok := c.freshRuntimeUpdateStatusEntry(lookup.key, now); ok {
		return entry
	}
	started := time.Now()
	result := c.runNativeUpdateCheck(ctx, lifetime, lookup)
	entry := newNativeUpdateCheckEntry(result, now)
	if c.logger != nil {
		c.logger.Debug("native update check finished",
			zap.String("agent", lookup.agentName), zap.String("state", string(result.State)),
			zap.String("reason", string(result.Reason)), zap.Duration("duration", time.Since(started)))
	}
	c.storeNativeUpdateCheckEntry(lookup, entry)
	return entry
}

func (c *Controller) runNativeUpdateCheck(ctx context.Context, lifetime *nativeCheckLifetime, lookup nativeUpdateCheckLookup) agents.NativeUpdateCheckResult {
	if lookup.executable == "" {
		return agents.NativeUpdateCheckResult{State: agents.NativeUpdateCheckStateUnknown, Reason: agents.NativeUpdateCheckReasonNotInstalled}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(lifetime.ctx, cancel)
	defer stop()
	run := c.nativeRunner().Run(runCtx, lookup.spec.Command)
	if run.Reason != "" {
		return agents.NativeUpdateCheckResult{State: agents.NativeUpdateCheckStateUnknown, Reason: run.Reason}
	}
	return normalizeNativeUpdateCheck(lookup.spec.Parse(run.Output), lookup.currentVersion)
}

// normalizeNativeUpdateCheck rejects a result about another installation than
// the one the agent runtime reports.
func normalizeNativeUpdateCheck(result agents.NativeUpdateCheckResult, runtimeVersion string) agents.NativeUpdateCheckResult {
	if result.CurrentVersion == "" || runtimeVersion == "" || sameRuntimeVersion(result.CurrentVersion, runtimeVersion) {
		return result
	}
	return agents.NativeUpdateCheckResult{State: agents.NativeUpdateCheckStateUnknown, Reason: agents.NativeUpdateCheckReasonRuntimeMismatch}
}

func sameRuntimeVersion(left, right string) bool {
	leftVersion, leftErr := semver.NewVersion(left)
	rightVersion, rightErr := semver.NewVersion(right)
	if leftErr == nil && rightErr == nil {
		return leftVersion.Equal(rightVersion)
	}
	return left == right
}

func nativeCancelledEntry() runtimeUpdateStatusCacheEntry {
	return newNativeUpdateCheckEntry(agents.NativeUpdateCheckResult{
		State: agents.NativeUpdateCheckStateUnknown, Reason: agents.NativeUpdateCheckReasonCancelled,
	}, time.Time{})
}

// newNativeUpdateCheckEntry applies the native TTLs. Cancelled and
// unavailable results have no expiry, so they are never cached.
func newNativeUpdateCheckEntry(result agents.NativeUpdateCheckResult, now time.Time) runtimeUpdateStatusCacheEntry {
	entry := runtimeUpdateStatusCacheEntry{
		native: true, state: dto.AgentUpdateCheckState(result.State),
		current: result.CurrentVersion, channel: result.Channel, reason: result.Reason,
	}
	if result.State == agents.NativeUpdateCheckStateUpdateAvailable {
		entry.latest = result.LatestVersion
	}
	switch {
	case result.Reason == agents.NativeUpdateCheckReasonCancelled, result.Reason == agents.NativeUpdateCheckReasonUnavailable:
	case result.State != agents.NativeUpdateCheckStateUnknown, result.Reason == agents.NativeUpdateCheckReasonUnsupported:
		entry.checkedAt, entry.expiresAt = now, now.Add(runtimeUpdateStatusSuccessTTL)
	default:
		entry.checkedAt, entry.expiresAt = now, now.Add(runtimeUpdateStatusFailureTTL)
	}
	return entry
}

// storeNativeUpdateCheckEntry writes only while the lookup's generation is
// current, so a check that overlapped an invalidation cannot repopulate it.
func (c *Controller) storeNativeUpdateCheckEntry(lookup nativeUpdateCheckLookup, entry runtimeUpdateStatusCacheEntry) {
	if entry.expiresAt.IsZero() {
		return
	}
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if c.nativeUpdateCheckGeneration[lookup.agentName] != lookup.generation {
		return
	}
	if c.runtimeUpdateStatusCache == nil {
		c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
	}
	c.runtimeUpdateStatusCache[lookup.key] = entry
}

// nativeUpdateCheckView is the DTO projection shared by status and preview.
type nativeUpdateCheckView struct {
	state     dto.AgentUpdateCheckState
	latest    string
	channel   string
	reason    dto.AgentUpdateCheckReason
	checkedAt *time.Time
}

func nativeUpdateCheckViewFor(entry runtimeUpdateStatusCacheEntry, eligible bool) nativeUpdateCheckView {
	view := nativeUpdateCheckView{state: dto.AgentUpdateCheckStateUnknown}
	if !eligible {
		view.reason = dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonUnavailable)
		return view
	}
	if !entry.native {
		return view
	}
	view.state, view.channel = entry.state, entry.channel
	if entry.state == dto.AgentUpdateCheckStateUpdateAvailable {
		view.latest = entry.latest
	}
	if entry.state == dto.AgentUpdateCheckStateUnknown {
		view.reason = dto.AgentUpdateCheckReason(entry.reason)
	}
	if !entry.checkedAt.IsZero() {
		checkedAt := entry.checkedAt
		view.checkedAt = &checkedAt
	}
	return view
}

func (v nativeUpdateCheckView) applyStatus(status *dto.AgentUpdateStatusDTO) {
	status.CheckSource = dto.AgentUpdateCheckSourceNative
	status.CheckState, status.LatestVersion = v.state, v.latest
	status.UpdateChannel, status.CheckReason, status.CheckedAt = v.channel, v.reason, v.checkedAt
}

func (v nativeUpdateCheckView) applyPreview(preview *dto.AgentUpdatePreviewDTO) {
	preview.CheckSource = dto.AgentUpdateCheckSourceNative
	preview.CheckState, preview.LatestVersion = v.state, v.latest
	preview.UpdateChannel, preview.CheckReason = v.channel, v.reason
}

// previewNativeUpdateCheck applies the eligibility guard with fresh discovery
// and then shares the status cache and single-flight execution.
func (c *Controller) previewNativeUpdateCheck(ctx context.Context, ag agents.Agent, spec agents.NativeUpdateCheckSpec, current string) nativeUpdateCheckView {
	available := map[string]bool{}
	if c.discovery != nil {
		results, err := c.detectAgents(ctx)
		if err != nil {
			return nativeUpdateCheckViewFor(runtimeUpdateStatusCacheEntry{}, false)
		}
		for _, result := range results {
			available[result.Name] = result.Available
		}
	}
	if !c.nativeUpdateCheckEligible(ag, available) {
		return nativeUpdateCheckViewFor(runtimeUpdateStatusCacheEntry{}, false)
	}
	lookup := c.prepareNativeUpdateCheck(ag.ID(), current, spec)
	return nativeUpdateCheckViewFor(c.nativeUpdateCheckEntry(ctx, lookup, c.runtimeUpdateStatusTime()), true)
}
