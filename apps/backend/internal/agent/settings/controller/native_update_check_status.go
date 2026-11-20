package controller

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	id     uint64
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	active int
	wg     sync.WaitGroup
}

var nativeCheckLifetimeIDs atomic.Uint64

func newNativeCheckLifetime() *nativeCheckLifetime {
	ctx, cancel := context.WithCancel(context.Background())
	return &nativeCheckLifetime{id: nativeCheckLifetimeIDs.Add(1), ctx: ctx, cancel: cancel}
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

// shut refuses new executions and cancels admitted ones without waiting.
func (l *nativeCheckLifetime) shut() {
	l.mu.Lock()
	l.closed = true
	l.cancel()
	l.mu.Unlock()
}

func (l *nativeCheckLifetime) close() {
	l.shut()
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
	// Active identities are kept so their epochs stay monotonic.
}

// DisableNativeUpdateCheckExecution makes every native check report
// not_installed without starting a process. Fixture backends whose release
// lookups are canned use it so status never runs a host executable or reaches
// a real release feed.
func (c *Controller) DisableNativeUpdateCheckExecution() {
	c.SetNativeUpdateCheckRunner(offlineNativeUpdateCheckRunner{})
}

type offlineNativeUpdateCheckRunner struct{}

func (offlineNativeUpdateCheckRunner) Resolve(agents.Command) (string, error) {
	return "", errNativeUpdateCheckOffline
}

func (offlineNativeUpdateCheckRunner) Environment() []string { return nil }

func (offlineNativeUpdateCheckRunner) Run(context.Context, agents.Command) nativeUpdateCheckRun {
	return nativeUpdateCheckRun{Reason: agents.NativeUpdateCheckReasonNotInstalled}
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
	c.deleteNativeEntriesLocked(prefix)
}

// deleteNativeEntriesLocked removes every cache entry with prefix. The caller
// holds runtimeUpdateStatusMu.
func (c *Controller) deleteNativeEntriesLocked(prefix string) {
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
	realExecutable string
	identity       string
	key            string
	generation     uint64
	spec           agents.NativeUpdateCheckSpec
	// abandoned is set when a bounded observation cannot complete; its result
	// is not shared or cached.
	abandoned       bool
	abandonedReason agents.NativeUpdateCheckReason
}

// nativeActiveIdentity is an agent's current native identity, the epoch at
// which it became current, and the sequence of the newest observation that
// confirmed it. The epoch increases on every identity change, so a return to
// an earlier identity gets a new cache and single-flight key.
type nativeActiveIdentity struct {
	identity string
	epoch    uint64
	observed uint64
}

func (a nativeActiveIdentity) key() string {
	return a.identity + "\x00e" + strconv.FormatUint(a.epoch, 10)
}

// nativeStaleObservationAttempts bounds re-observation when newer
// observations keep committing first.
const nativeStaleObservationAttempts = 3

// prepareNativeUpdateCheck computes the cache identity without starting a
// process and makes it the agent's active identity. Switching identity evicts
// the agent's other entries and advances its epoch, so returning to an
// earlier identity rechecks instead of reusing or joining an older check. An
// observation that started before the one that committed the active identity
// never replaces it; it observes again, and after repeated losses it uses an
// isolated key that is neither shared nor cached.
func (c *Controller) prepareNativeUpdateCheck(ctx context.Context, agentName string, spec agents.NativeUpdateCheckSpec) nativeUpdateCheckLookup {
	lifetime := c.currentNativeCheckLifetime()
	observationCtx, cancel := context.WithTimeout(ctx, runtimeUpdateStatusLookupTimeout)
	defer cancel()
	for attempt := 1; ; attempt++ {
		seq := c.nextNativeObservation()
		currentVersion := c.observedRuntimeVersion(agentName)
		lookup, ok := c.observeNativeUpdateCheckIdentity(observationCtx, lifetime, agentName, currentVersion, spec)
		if !ok {
			return nativeUpdateCheckLookup{
				agentName: agentName, currentVersion: currentVersion, spec: spec, abandoned: true,
				abandonedReason: nativeUpdateCheckObservationAbortReason(observationCtx),
				key:             nativeUpdateCheckKeyPrefix(agentName) + "abandoned" + strconv.FormatUint(seq, 10),
			}
		}
		if key, ok := c.commitNativeObservation(agentName, lookup.identity, seq); ok {
			lookup.key = key
			return lookup
		}
		if attempt == nativeStaleObservationAttempts {
			lookup.key = lookup.identity + "\x00stale" + strconv.FormatUint(seq, 10)
			return lookup
		}
	}
}

// observeNativeUpdateCheckIdentity bounds filesystem reads that cannot be
// interrupted. Cancellation stops the caller's wait while the read keeps its
// slot until the underlying operation returns.
func (c *Controller) observeNativeUpdateCheckIdentity(ctx context.Context, lifetime *nativeCheckLifetime, agentName, currentVersion string, spec agents.NativeUpdateCheckSpec) (nativeUpdateCheckLookup, bool) {
	if ctx.Err() != nil || lifetime.isClosed() {
		return nativeUpdateCheckLookup{}, false
	}
	slots := c.nativeUpdateCheckIdentityReadSlots()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nativeUpdateCheckLookup{}, false
	case <-lifetime.ctx.Done():
		return nativeUpdateCheckLookup{}, false
	}
	if ctx.Err() != nil || lifetime.isClosed() {
		<-slots
		return nativeUpdateCheckLookup{}, false
	}
	observed := make(chan nativeUpdateCheckLookup, 1)
	go func() {
		defer func() { <-slots }()
		observed <- c.nativeUpdateCheckIdentity(agentName, currentVersion, spec)
	}()
	select {
	case lookup := <-observed:
		return lookup, true
	case <-ctx.Done():
		return nativeUpdateCheckLookup{}, false
	case <-lifetime.ctx.Done():
		return nativeUpdateCheckLookup{}, false
	}
}

func (c *Controller) nativeUpdateCheckIdentityReadSlots() chan struct{} {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if c.nativeUpdateCheckIdentityReads == nil {
		c.nativeUpdateCheckIdentityReads = make(chan struct{}, runtimeUpdateStatusMaxConcurrent)
	}
	return c.nativeUpdateCheckIdentityReads
}

func nativeUpdateCheckObservationAbortReason(ctx context.Context) agents.NativeUpdateCheckReason {
	if ctx.Err() == context.DeadlineExceeded {
		return agents.NativeUpdateCheckReasonTimeout
	}
	return agents.NativeUpdateCheckReasonCancelled
}

func (c *Controller) nextNativeObservation() uint64 {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	c.nativeUpdateCheckObserved++
	return c.nativeUpdateCheckObserved
}

// commitNativeObservation records an observation and returns the cache key,
// or false when the observation is older than the active identity's newest
// observation and disagrees with it.
func (c *Controller) commitNativeObservation(agentName, identity string, seq uint64) (string, bool) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	active := c.nativeUpdateCheckActive[agentName]
	switch {
	case active.identity == identity:
		active.observed = max(active.observed, seq)
	case seq > active.observed:
		c.deleteNativeEntriesLocked(nativeUpdateCheckKeyPrefix(agentName))
		active = nativeActiveIdentity{identity: identity, epoch: active.epoch + 1, observed: seq}
	default:
		return "", false
	}
	if c.nativeUpdateCheckActive == nil {
		c.nativeUpdateCheckActive = make(map[string]nativeActiveIdentity)
	}
	c.nativeUpdateCheckActive[agentName] = active
	return active.key(), true
}

// observedRuntimeVersion reads the runtime-reported version as part of an
// observation, so the version and the other identity inputs are snapshotted
// together.
func (c *Controller) observedRuntimeVersion(agentName string) string {
	if c.runtimeUpdater == nil {
		return ""
	}
	if caps, found := c.runtimeUpdater.CurrentCapabilities(agentName); found {
		return caps.AgentVersion
	}
	return ""
}

// nativeUpdateCheckIdentity resolves the launcher and real executable,
// runtime version, generation, and update configuration fingerprint into one
// identity. The cache key adds the agent's identity epoch.
func (c *Controller) nativeUpdateCheckIdentity(agentName, currentVersion string, spec agents.NativeUpdateCheckSpec) nativeUpdateCheckLookup {
	runner := c.nativeRunner()
	executable, err := runner.Resolve(spec.Command)
	if err != nil {
		executable = ""
	}
	workingDir, _ := os.Getwd()
	// OMP resolves relative paths against process.cwd(), the physical
	// directory; os.Getwd may return a logical $PWD through a symlink.
	if physical, err := filepath.EvalSymlinks(workingDir); err == nil {
		workingDir = physical
	}
	resolver := agents.NewNativeUpdateCheckResolver(runner.Environment(), workingDir, executable)
	fingerprint := spec.Context.Fingerprint(resolver)
	c.runtimeUpdateStatusMu.Lock()
	generation := c.nativeUpdateCheckGeneration[agentName]
	c.runtimeUpdateStatusMu.Unlock()
	return nativeUpdateCheckLookup{
		agentName: agentName, currentVersion: currentVersion, executable: executable,
		realExecutable: resolver.Executable, generation: generation, spec: spec,
		identity: nativeUpdateCheckCacheKey(agentName, generation, spec.Command.Args(),
			resolver.Launcher, resolver.Executable+"\x00"+nativeExecutableStat(resolver.Executable), currentVersion, fingerprint),
	}
}

// nativeExecutableStat identifies the executable file's content without
// reading it: size, modification time, and mode. A same-path replacement,
// as OMP's own updater performs, changes it.
func nativeExecutableStat(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	return strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + info.Mode().String()
}

func nativeUpdateCheckKeyPrefix(agentName string) string {
	return "native\x00" + agentName + "\x00"
}

func nativeUpdateCheckCacheKey(agentName string, generation uint64, argv []string, launcher, executable, version, fingerprint string) string {
	parts := make([]string, 0, len(argv)+5)
	parts = append(parts, strconv.FormatUint(generation, 10), strconv.Itoa(len(argv)))
	parts = append(parts, argv...)
	parts = append(parts, launcher, executable, version, fingerprint)
	return nativeUpdateCheckKeyPrefix(agentName) + strings.Join(parts, "\x00")
}

// nativeUpdateCheckEntry returns a cached result or joins the single shared
// execution for the lookup key within the current lifetime. The flight key
// includes the lifetime, so after a stop and restart no request joins a
// cancelled flight that has not published yet; the cache key does not.
func (c *Controller) nativeUpdateCheckEntry(ctx context.Context, lookup nativeUpdateCheckLookup, now time.Time) runtimeUpdateStatusCacheEntry {
	if ctx.Err() != nil {
		return runtimeUpdateStatusCacheEntry{}
	}
	if lookup.abandoned {
		return nativeUnknownUpdateCheckEntry(lookup.abandonedReason)
	}
	if entry, ok := c.freshRuntimeUpdateStatusEntry(lookup.key, now); ok {
		return entry
	}
	lifetime := c.currentNativeCheckLifetime()
	shared := context.WithoutCancel(ctx)
	flightKey := lookup.key + "\x00l" + strconv.FormatUint(lifetime.id, 10)
	result := c.runtimeUpdateStatusFlight.DoChan(flightKey, func() (interface{}, error) {
		executionCtx, cancel := context.WithTimeoutCause(shared, UpdateStatusResponseBudget, errNativeUpdateCheckTimeout)
		defer cancel()
		entry := c.executeNativeUpdateCheck(executionCtx, lifetime, lookup, now)
		if hook := c.nativeCheckBeforePublish; hook != nil {
			hook()
		}
		return entry, nil
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

func (c *Controller) executeNativeUpdateCheck(ctx context.Context, lifetime *nativeCheckLifetime, lookup nativeUpdateCheckLookup, now time.Time) runtimeUpdateStatusCacheEntry {
	if !lifetime.admit() {
		return nativeCancelledEntry()
	}
	defer lifetime.done()
	slots := c.runtimeUpdateStatusSemaphore()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nativeUpdateCheckContextEntry(ctx)
	case <-lifetime.ctx.Done():
		return nativeCancelledEntry()
	}
	defer func() { <-slots }()
	if entry, ok := c.freshRuntimeUpdateStatusEntry(lookup.key, now); ok {
		return entry
	}
	started := time.Now()
	result, launched := c.runNativeUpdateCheck(ctx, lifetime, lookup)
	entry := newNativeUpdateCheckEntry(result, now)
	if c.logger != nil {
		c.logger.Debug("native update check finished",
			zap.String("agent", lookup.agentName), zap.String("state", string(result.State)),
			zap.String("reason", string(result.Reason)), zap.Duration("duration", time.Since(started)))
	}
	// An uncacheable entry (cancelled, unavailable) returns at once: the
	// post-run identity does filesystem I/O that must not delay shutdown.
	// A result whose executable or configuration changed during the wait or
	// the run describes another identity and is returned without caching. The
	// launched launcher and its real executable at launch must be the
	// prepared ones, and the identity observed after the run must still match.
	// If shutdown abandons that observation, the decided result is returned
	// uncached.
	if entry.expiresAt.IsZero() || launched.Executable != lookup.executable || launched.RealExecutable != lookup.realExecutable {
		return entry
	}
	observationCtx, cancel := context.WithTimeout(ctx, runtimeUpdateStatusLookupTimeout)
	after, observed := c.observeNativeUpdateCheckIdentity(observationCtx, lifetime, lookup.agentName, lookup.currentVersion, lookup.spec)
	cancel()
	if observed && after.identity == lookup.identity {
		c.storeNativeUpdateCheckEntry(lookup, entry)
	}
	return entry
}

// runNativeUpdateCheck returns the normalized result and the run, whose
// Executable and RealExecutable identify what was launched (empty when
// nothing was).
func (c *Controller) runNativeUpdateCheck(ctx context.Context, lifetime *nativeCheckLifetime, lookup nativeUpdateCheckLookup) (agents.NativeUpdateCheckResult, nativeUpdateCheckRun) {
	if lookup.executable == "" {
		return agents.NativeUpdateCheckResult{State: agents.NativeUpdateCheckStateUnknown, Reason: agents.NativeUpdateCheckReasonNotInstalled}, nativeUpdateCheckRun{}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(lifetime.ctx, cancel)
	defer stop()
	run := c.nativeRunner().Run(runCtx, lookup.spec.Command)
	if run.Reason != "" {
		return agents.NativeUpdateCheckResult{State: agents.NativeUpdateCheckStateUnknown, Reason: run.Reason}, run
	}
	return normalizeNativeUpdateCheck(lookup.spec.Parse(run.Output), lookup.currentVersion), run
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
	return nativeUnknownUpdateCheckEntry(agents.NativeUpdateCheckReasonCancelled)
}

func nativeUnknownUpdateCheckEntry(reason agents.NativeUpdateCheckReason) runtimeUpdateStatusCacheEntry {
	if reason == "" {
		reason = agents.NativeUpdateCheckReasonCancelled
	}
	return runtimeUpdateStatusCacheEntry{native: true, state: dto.AgentUpdateCheckStateUnknown, reason: reason}
}

func nativeUpdateCheckContextEntry(ctx context.Context) runtimeUpdateStatusCacheEntry {
	reason := nativeUpdateCheckContextReason(context.Background(), ctx)
	return nativeUnknownUpdateCheckEntry(reason)
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

// storeNativeUpdateCheckEntry writes only while the lookup's generation and
// identity are current, so an overlapping invalidation or identity change
// cannot repopulate the cache.
func (c *Controller) storeNativeUpdateCheckEntry(lookup nativeUpdateCheckLookup, entry runtimeUpdateStatusCacheEntry) {
	if entry.expiresAt.IsZero() {
		return
	}
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if c.nativeUpdateCheckGeneration[lookup.agentName] != lookup.generation ||
		c.nativeUpdateCheckActive[lookup.agentName].key() != lookup.key {
		return
	}
	if c.runtimeUpdateStatusCache == nil {
		c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
	}
	c.runtimeUpdateStatusCache[lookup.key] = entry
}

// nativeUpdateCheckView is the DTO projection shared by status and preview.
// observed reports whether runtimeVersion is the version the accepted
// observation used.
type nativeUpdateCheckView struct {
	state          dto.AgentUpdateCheckState
	latest         string
	current        string
	channel        string
	reason         dto.AgentUpdateCheckReason
	checkedAt      *time.Time
	runtimeVersion string
	observed       bool
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
	view.state, view.channel, view.current = entry.state, entry.channel, entry.current
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

// The check-reported current version fills CurrentVersion only when the
// runtime has not reported one; EffectiveVersion stays runtime-owned.
func (v nativeUpdateCheckView) applyStatus(status *dto.AgentUpdateStatusDTO) {
	status.CheckSource = dto.AgentUpdateCheckSourceNative
	status.CheckState, status.LatestVersion = v.state, v.latest
	status.UpdateChannel, status.CheckReason, status.CheckedAt = v.channel, v.reason, v.checkedAt
	if status.CurrentVersion == "" {
		status.CurrentVersion = v.current
	}
}

// When the preview made an observation, its runtime version is projected and
// drives the operation, matching the version that keyed the result.
func (v nativeUpdateCheckView) applyPreview(preview *dto.AgentUpdatePreviewDTO) {
	preview.CheckSource = dto.AgentUpdateCheckSourceNative
	preview.CheckState, preview.LatestVersion = v.state, v.latest
	preview.UpdateChannel, preview.CheckReason = v.channel, v.reason
	if v.observed {
		preview.CurrentVersion, preview.EffectiveVersion = v.runtimeVersion, v.runtimeVersion
		preview.Operation = harnessUpdateOperation(v.runtimeVersion)
	}
	if preview.CurrentVersion == "" {
		preview.CurrentVersion = v.current
	}
}

// previewNativeUpdateCheck applies the eligibility guard with fresh discovery
// and then shares the status cache and single-flight execution.
func (c *Controller) previewNativeUpdateCheck(ctx context.Context, ag agents.Agent, spec agents.NativeUpdateCheckSpec) nativeUpdateCheckView {
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
	lookup := c.prepareNativeUpdateCheck(ctx, ag.ID(), spec)
	view := nativeUpdateCheckViewFor(c.nativeUpdateCheckEntry(ctx, lookup, c.runtimeUpdateStatusTime()), true)
	view.runtimeVersion, view.observed = lookup.currentVersion, true
	return view
}
