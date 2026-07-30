package orchestrator

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// turnActivity tracks foreground ownership and spawned background-work liveness
// independently. Foreground activity always has precedence; background work is
// visible only after an explicit foreground-idle boundary.
//
// It is a best-effort accounting signal only. The default admission and public
// activity contracts never trust it. A high-risk, default-off experiment may
// use it for persisted Claude Code sessions, whose adapter supplies the
// recognized lifecycle attestations below.
//
// The absent/zero internal state means "foreground generating". A recognized
// registration plus an explicit idle boundary can move only this private
// estimate to background.
type turnActivity struct {
	mu            sync.Mutex
	revision      uint64                    // invalidates delayed publications after newer mutations
	background    map[string]backgroundWork // outstanding work keyed by launch tool-call ID
	tools         map[string]toolOwnershipRecord
	yielded       bool   // foreground handed off to background work
	backgroundSeq uint64 // next sequence number for a new background registration

	// executionClaims records every execution that has positively claimed this
	// record through prompt dispatch, tool ownership, or background registration.
	// Terminal cleanup removes its own claim and detaches the record only when no
	// other execution or in-flight prompt token still uses it.
	executionClaims map[string]struct{}

	// dispatchInFlight protects the record between prompt-cycle creation and
	// agentctl's accept/rollback callback. A successor can hold this pointer
	// before its execution emits a stream frame; teardown must treat that token
	// as live use even when no execution ID was available at admission time.
	dispatchInFlightGeneration uint64
	// retireWhenUnused records that terminal teardown found no execution-owned
	// state but could not detach this record because an admission/dispatch token
	// still held its pointer. Releasing the last token must finish retirement.
	retireWhenUnused bool

	// promptInFlight marks an admitted prompt that has claimed the foreground turn
	// but has not yet been handed to the agent. It is deliberately independent of
	// `yielded`: during that window the session must stay un-promptable no matter
	// what happens to the background set. Otherwise a background tool_call landing
	// mid-admission (registerBackgroundWork re-sets `yielded`) would reopen the gate
	// under the in-flight prompt and let a second one through — two prompts reaching
	// one ACP session, which is the exact overlap the claim exists to prevent.
	promptInFlight  bool
	claimGeneration uint64 // identifies the current admission owner
	foregroundEpoch uint64 // increments on genuine foreground output

	// promptCycleGeneration identifies the accepted prompt cycle that currently
	// owns the foreground. It advances only when a successor prompt is accepted;
	// output resuming after an idle frame remains part of the same immutable cycle.
	// Provider-idle records this generation so a delayed predecessor completion
	// cannot yield a successor cycle that has since claimed the same session.
	promptCycleGeneration          uint64
	pendingCompletionGeneration    uint64
	hasPendingCompletionGeneration bool
	completedGeneration            uint64
	hasCompletedGeneration         bool
}

// backgroundWork binds a detached workload to the execution that launched it.
// workID is a provider-owned stable identity when the launch envelope exposes
// one (Claude's async Task result calls this agentId). The current Claude ACP
// task-notification usage frame exposes only origin.kind, so workID is often
// available at launch but absent at completion.
type backgroundWork struct {
	executionID string
	workID      string
	kind        streams.BackgroundWorkKind
	// seq is a monotonically increasing per-session registration order,
	// assigned only when a registration is first created. It lets an
	// uncorrelated (ID-less) completion retire the oldest outstanding
	// registration deterministically instead of guessing via Go's
	// intentionally-random map iteration order.
	seq uint64
}

type toolOwnership uint8

const (
	toolOwnershipUnknown toolOwnership = iota
	toolOwnershipForeground
	toolOwnershipBackground
	toolOwnershipChild
)

// toolOwnershipRecord binds the initial tool-call classification to the
// execution that supplied it. Provider-local tool IDs may be reused after an
// execution rotates, so lookups and terminal clears must match both identities.
type toolOwnershipRecord struct {
	executionID string
	ownership   toolOwnership
}

type activityPublication struct {
	activity            *turnActivity
	revision            uint64
	value               interface{}
	activeSubagentCount int
	retired             bool
}

type activityPublicationGuard struct {
	mu   sync.Mutex
	refs int
}

type executionActivityRetirement struct {
	removed         bool
	removedSubagent bool
	finalTransition bool
	retireRecord    bool
}

type foregroundYieldSource uint8

const (
	foregroundYieldProviderIdle foregroundYieldSource = iota
	foregroundYieldTurnCompletion
)

// foregroundClaim binds admission to the exact activity record and generation
// it claimed. The foreground epoch separately detects output that makes a failed
// prompt's background-idle restoration stale.
type foregroundClaim struct {
	sessionID       string
	activity        *turnActivity
	claimGeneration uint64
	foregroundEpoch uint64
}

// foregroundDispatch binds provider events to a prompt cycle before the
// provider can emit them. Its generation is also the rollback token: a failed
// dispatch may restore the predecessor only while this exact, unobserved cycle
// is still current.
type foregroundDispatch struct {
	sessionID             string
	activity              *turnActivity
	generation            uint64
	foregroundEpoch       uint64
	claimGeneration       uint64
	claimedBackgroundTurn bool
	yieldedBeforeBegin    bool
	accepted              bool
}

// lockTurnActivity couples record lookup with the record lock. The map guard is
// not released until the selected record is locked, so execution retirement
// cannot detach a record after a successor loaded its pointer but before that
// successor begins mutating it. Every mapped-record access must use this helper
// (or lockTurnActivityToken for a previously-issued claim/dispatch token).
func (s *Service) lockTurnActivity(sessionID string, create bool) *turnActivity {
	s.foregroundActivityMu.Lock()
	if ta, ok := s.foregroundActivity[sessionID]; ok {
		ta.mu.Lock()
		s.foregroundActivityMu.Unlock()
		return ta
	}
	if !create {
		s.foregroundActivityMu.Unlock()
		return nil
	}
	ta := &turnActivity{
		background:      make(map[string]backgroundWork),
		tools:           make(map[string]toolOwnershipRecord),
		executionClaims: make(map[string]struct{}),
	}
	if s.foregroundActivity == nil {
		s.foregroundActivity = make(map[string]*turnActivity)
	}
	s.foregroundActivity[sessionID] = ta
	ta.mu.Lock()
	s.foregroundActivityMu.Unlock()
	return ta
}

func (s *Service) lockTurnActivityToken(sessionID string, ta *turnActivity) bool {
	if sessionID == "" || ta == nil {
		return false
	}
	s.foregroundActivityMu.Lock()
	current, ok := s.foregroundActivity[sessionID]
	if !ok || current != ta {
		s.foregroundActivityMu.Unlock()
		return false
	}
	ta.mu.Lock()
	s.foregroundActivityMu.Unlock()
	return true
}

// acquireActivityPublicationGuard returns a stable per-session publication
// mutex whose identity outlives any individual turnActivity record. Holding it
// across snapshot validation and synchronous event delivery gives publications
// from predecessor and successor record generations one linearization boundary:
// either predecessor delivery completes first, or successor mutation invalidates
// it before it can publish. The registry itself stays bounded by active users.
func (s *Service) acquireActivityPublicationGuard(sessionID string) (*sync.Mutex, func()) {
	s.activityPublicationGuardsMu.Lock()
	guard, ok := s.activityPublicationGuards[sessionID]
	if !ok {
		guard = &activityPublicationGuard{}
		if s.activityPublicationGuards == nil {
			s.activityPublicationGuards = make(map[string]*activityPublicationGuard)
		}
		s.activityPublicationGuards[sessionID] = guard
	}
	guard.refs++
	s.activityPublicationGuardsMu.Unlock()

	var released bool
	release := func() {
		if released {
			return
		}
		released = true
		s.activityPublicationGuardsMu.Lock()
		guard.refs--
		if guard.refs == 0 {
			delete(s.activityPublicationGuards, sessionID)
		}
		s.activityPublicationGuardsMu.Unlock()
	}
	return &guard.mu, release
}

func (s *Service) recordToolOwnership(
	sessionID, toolCallID, executionID string,
	ownership toolOwnership,
) {
	if sessionID == "" || toolCallID == "" || ownership == toolOwnershipUnknown {
		return
	}
	ta := s.lockTurnActivity(sessionID, true)
	defer ta.mu.Unlock()
	if ta.tools == nil {
		ta.tools = make(map[string]toolOwnershipRecord)
	}
	ta.claimExecutionLocked(executionID)
	ta.tools[toolCallID] = toolOwnershipRecord{
		executionID: executionID,
		ownership:   ownership,
	}
}

func (s *Service) toolOwnership(sessionID, toolCallID, executionID string) toolOwnership {
	if toolCallID == "" {
		return toolOwnershipUnknown
	}
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return toolOwnershipUnknown
	}
	defer ta.mu.Unlock()
	record := ta.tools[toolCallID]
	if record.executionID != executionID {
		return toolOwnershipUnknown
	}
	return record.ownership
}

func (s *Service) clearToolOwnership(sessionID, toolCallID, executionID string) {
	if toolCallID == "" {
		return
	}
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return
	}
	defer ta.mu.Unlock()
	if record := ta.tools[toolCallID]; record.executionID == executionID {
		delete(ta.tools, toolCallID)
	}
}

func (ta *turnActivity) claimExecutionLocked(executionID string) {
	if executionID == "" {
		return
	}
	if _, exists := ta.executionClaims[executionID]; exists {
		return
	}
	ta.executionClaims[executionID] = struct{}{}
	ta.revision++
}

func (ta *turnActivity) retireExecutionClaimLocked(executionID string) bool {
	if _, exists := ta.executionClaims[executionID]; !exists {
		return false
	}
	delete(ta.executionClaims, executionID)
	return true
}

// markForegroundGenerating records that the foreground agent produced output
// (streamed a message/thinking chunk, or a fresh foreground prompt was
// dispatched), so the turn is once again driven by the foreground even if a
// background task is still outstanding. It returns true when this call actually
// flipped the session out of the background-idle substate, so the caller can
// publish the operator-facing activity signal only on a real transition.
func (s *Service) markForegroundGenerating(sessionID string, executionIDs ...string) bool {
	if sessionID == "" {
		return false
	}
	ta := s.lockTurnActivity(sessionID, true)
	defer ta.mu.Unlock()
	if len(executionIDs) > 0 {
		ta.claimExecutionLocked(executionIDs[0])
	}
	changed := ta.yielded
	ta.yielded = false
	if changed {
		ta.revision++
	}
	// Real foreground output redefines ownership of the turn: it invalidates any
	// outstanding claim's epoch, so a prompt that later fails cannot release the
	// gate back open on top of a foreground that is now genuinely generating.
	ta.foregroundEpoch++
	return changed
}

// markForegroundIdle records an explicit provider boundary proving the current
// foreground model cycle ended. Outstanding background work becomes the visible
// activity only when no prompt admission claim is still in flight.
func (s *Service) markForegroundIdle(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return false
	}
	defer ta.mu.Unlock()
	return ta.markForegroundIdleLocked()
}

func (ta *turnActivity) markForegroundIdleLocked() bool {
	if ta.promptInFlight || len(ta.background) == 0 || ta.yielded {
		return false
	}
	ta.yielded = true
	ta.revision++
	return true
}

// yieldForegroundAndPublish records the foreground-to-background transition and
// publishes both operator-facing activity signals exactly once. taskID may be
// omitted by lifecycle callers that only have a session ID. Resolve identity
// before touching the tracker so a transient lookup failure cannot consume an
// otherwise publishable transition. The source-specific generation check and
// mutation happen atomically; publication happens after the lock is released.
func (s *Service) yieldForegroundAndPublish(
	ctx context.Context,
	taskID, sessionID string,
	source foregroundYieldSource,
) {
	// No turnActivity record means transitionForegroundToBackground below is
	// guaranteed to no-op (it also bails on a nil record), so every turn close
	// for an untracked session would otherwise pay a wasted GetTaskSession read
	// (and risk a spurious Warn on failure) for a transition that can never
	// publish. Bail before that read.
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return
	}
	ta.mu.Unlock()
	if taskID == "" {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil {
			s.logger.Warn("resolve task for foreground activity publish failed",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
		taskID = session.TaskID
	}
	if !s.transitionForegroundToBackground(sessionID, source) {
		return
	}
	s.publishForegroundActivityChanged(ctx, taskID, sessionID)
}

func (s *Service) transitionForegroundToBackground(sessionID string, source foregroundYieldSource) bool {
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return false
	}
	defer ta.mu.Unlock()

	if source == foregroundYieldProviderIdle {
		if !ta.hasCompletedGeneration || ta.completedGeneration != ta.promptCycleGeneration {
			ta.pendingCompletionGeneration = ta.promptCycleGeneration
			ta.hasPendingCompletionGeneration = true
		}
		return ta.markForegroundIdleLocked()
	}

	completionGeneration := ta.promptCycleGeneration
	if ta.hasPendingCompletionGeneration {
		completionGeneration = ta.pendingCompletionGeneration
		ta.hasPendingCompletionGeneration = false
	}
	if ta.hasCompletedGeneration && ta.completedGeneration == completionGeneration {
		return false
	}
	ta.completedGeneration = completionGeneration
	ta.hasCompletedGeneration = true
	if completionGeneration != ta.promptCycleGeneration {
		return false
	}
	return ta.markForegroundIdleLocked()
}

// registerBackgroundWork records a spawned background task (a subagent Task or
// a run-in-background shell). Registration alone is not evidence that the
// foreground yielded: launch frames can arrive while the top-level agent is
// still generating, and foreground activity must retain precedence. A later
// foreground-idle boundary exposes the outstanding work.
//
// Callers should pass the launching execution/work IDs whenever they are
// already known (e.g. from the initial tool_call event) rather than relying on
// a later update to backfill them: hasBackgroundTask short-circuits
// registration once the tool_call_id is tracked, so an empty executionID can
// never be filled in afterward, and retireExecutionActivitySnapshot can
// then never retire the entry on execution teardown.
func (s *Service) registerBackgroundWork(sessionID, toolCallID, executionID, workID string) {
	s.registerBackgroundWorkKind(sessionID, toolCallID, executionID, workID, "")
}

func (s *Service) registerBackgroundWorkKind(
	sessionID, toolCallID, executionID, workID string,
	kind streams.BackgroundWorkKind,
) bool {
	if sessionID == "" || toolCallID == "" {
		return false
	}
	ta := s.lockTurnActivity(sessionID, true)
	defer ta.mu.Unlock()
	ta.claimExecutionLocked(executionID)
	current, exists := ta.background[toolCallID]
	updated := current
	if executionID != "" {
		updated.executionID = executionID
	}
	if workID != "" {
		updated.workID = workID
	}
	if kind != "" {
		updated.kind = kind
	}
	if !exists {
		// Assign a sequence only for a brand-new entry: an update that merely
		// backfills executionID/workID on an existing registration must keep
		// its original seq, so FIFO retirement order reflects true
		// registration order rather than the most recent backfill.
		ta.backgroundSeq++
		updated.seq = ta.backgroundSeq
	}
	changed := !exists || current != updated
	if changed {
		ta.revision++
	}
	ta.background[toolCallID] = updated
	return changed
}

// ActiveSubagentCount derives the number of live adapter-attested subagents
// from the registration map. It is not maintained as an independent counter,
// so duplicate and out-of-order completion cannot make it drift.
func (s *Service) ActiveSubagentCount(sessionID string) int {
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return 0
	}
	defer ta.mu.Unlock()
	return ta.activeSubagentCountLocked()
}

func (ta *turnActivity) activeSubagentCountLocked() int {
	count := 0
	for _, work := range ta.background {
		if work.kind == streams.BackgroundWorkKindSubagent {
			count++
		}
	}
	return count
}

// hasBackgroundTask reports whether toolCallID is already tracked as outstanding
// background work for the session. Used to make the tool_call_update
// registration path fire only on the first recognizable frame: re-registering
// on later updates would re-set `yielded` and clobber a foreground stream that
// marked the turn generating again (see markForegroundGenerating).
func (s *Service) hasBackgroundTask(sessionID, toolCallID string, executionIDs ...string) bool {
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return false
	}
	defer ta.mu.Unlock()
	work, ok := ta.background[toolCallID]
	if !ok || len(executionIDs) == 0 || executionIDs[0] == "" {
		return ok
	}
	return work.executionID == executionIDs[0]
}

// completeBackgroundTaskForExecution clears a previously-registered background
// task, scoped to the execution that owns it (an empty executionID matches any
// owner — see the test-only completeBackgroundTask wrapper). When no
// background task remains, the foreground turn is no longer "waiting on
// background". It returns true whenever a registration was removed so the
// caller also publishes count-only transitions while other work remains.
func (s *Service) completeBackgroundTaskForExecution(sessionID, toolCallID, executionID string) bool {
	if sessionID == "" || toolCallID == "" {
		return false
	}
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return false
	}
	defer ta.mu.Unlock()
	work, exists := ta.background[toolCallID]
	if !exists || (executionID != "" && work.executionID != "" && work.executionID != executionID) {
		return false
	}
	delete(ta.background, toolCallID)
	ta.revision++
	visibleChanged := false
	if len(ta.background) == 0 && ta.yielded {
		ta.yielded = false
		visibleChanged = true
	}
	return visibleChanged || work.kind == streams.BackgroundWorkKindSubagent
}

// completeBackgroundWork retires a provider completion. Identified completions
// remove only their exact execution-scoped registration, making duplicate
// delivery harmless. An ID-less completion retires exactly one outstanding
// registration — the oldest by registration order (FIFO) — and leaves any
// remainder live: Claude attributes an ID-less task-notification to the ACP
// cycle that receives it, not necessarily the execution/cycle that launched
// the async child, so there is no way to attribute it to a specific workload.
// Retiring the oldest deterministically drains one registration per
// completion instead of leaving every registration stuck forever whenever
// more than one workload is outstanding. In particular, never range a Go map
// and pretend its intentionally-random iteration order is completion
// ordering — use the tracked seq instead.
func (s *Service) completeBackgroundWorkSnapshot(
	sessionID, executionID, workID string,
	value interface{},
) (activityPublication, bool) {
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return activityPublication{}, false
	}
	defer ta.mu.Unlock()

	matchedToolCallID := ""
	oldestSeq := uint64(0)
	for toolCallID, work := range ta.background {
		if workID != "" {
			// Identified completion remains execution-scoped: a delayed exact event
			// from an old execution must never consume similarly-identified work
			// owned by its successor.
			if executionID != "" && work.executionID != "" && work.executionID != executionID {
				continue
			}
			if work.workID == workID || toolCallID == workID {
				matchedToolCallID = toolCallID
				break
			}
			continue
		}
		// Uncorrelated completion: track the oldest (lowest seq) candidate seen
		// so far so the final choice is FIFO regardless of map iteration order.
		if matchedToolCallID == "" || work.seq < oldestSeq {
			matchedToolCallID = toolCallID
			oldestSeq = work.seq
		}
	}
	if matchedToolCallID == "" {
		return activityPublication{}, false
	}
	removed := ta.background[matchedToolCallID]
	delete(ta.background, matchedToolCallID)
	ta.revision++
	visibleChanged := ta.clearYieldWhenEmptyLocked()
	if !visibleChanged && removed.kind != streams.BackgroundWorkKindSubagent {
		return activityPublication{}, false
	}
	publicationValue := value
	if !visibleChanged && ta.yielded {
		publicationValue = string(v1.ForegroundActivityBackground)
	}
	return activityPublication{
		activity:            ta,
		revision:            ta.revision,
		value:               publicationValue,
		activeSubagentCount: ta.activeSubagentCountLocked(),
	}, true
}

// retireExecutionActivitySnapshot retires all activity owned by one
// terminal execution. Record publication, identity validation, and detachment
// share activityPublicationGuard -> foregroundActivityMu -> turnActivity.mu
// lock order.
//
// The map/record lock coupling is the critical invariant: a successor either
// locks this mapped record before cleanup can detach it, or observes no record
// and creates a new one. Cleanup therefore cannot detach a pointer that a
// successor has loaded but not yet begun mutating. A prompt claim or dispatch
// token also counts as live use until it is released.
func (s *Service) retireExecutionActivitySnapshot(
	sessionID, executionID string,
) (activityPublication, bool) {
	if sessionID == "" || executionID == "" {
		return activityPublication{}, false
	}
	publicationLock, releasePublication := s.acquireActivityPublicationGuard(sessionID)
	defer releasePublication()
	publicationLock.Lock()
	defer publicationLock.Unlock()
	s.foregroundActivityMu.Lock()
	ta, ok := s.foregroundActivity[sessionID]
	if !ok {
		s.foregroundActivityMu.Unlock()
		return activityPublication{}, false
	}
	ta.mu.Lock()

	retirement := ta.retireExecutionActivityLocked(executionID)
	if retirement.retireRecord {
		delete(s.foregroundActivity, sessionID)
		publication := activityPublication{
			activity:            ta,
			revision:            ta.revision,
			value:               nil,
			activeSubagentCount: 0,
			retired:             true,
		}
		ta.mu.Unlock()
		s.foregroundActivityMu.Unlock()
		return publication, true
	}

	if !retirement.removed {
		ta.mu.Unlock()
		s.foregroundActivityMu.Unlock()
		return activityPublication{}, false
	}
	if !retirement.finalTransition && !retirement.removedSubagent {
		ta.mu.Unlock()
		s.foregroundActivityMu.Unlock()
		return activityPublication{}, false
	}
	publicationValue := interface{}(string(v1.ForegroundActivityGenerating))
	if ta.yielded {
		publicationValue = string(v1.ForegroundActivityBackground)
	} else if retirement.finalTransition {
		publicationValue = nil
	}
	publication := activityPublication{
		activity:            ta,
		revision:            ta.revision,
		value:               publicationValue,
		activeSubagentCount: ta.activeSubagentCountLocked(),
	}
	ta.mu.Unlock()
	s.foregroundActivityMu.Unlock()
	return publication, true
}

func (ta *turnActivity) retireExecutionActivityLocked(
	executionID string,
) executionActivityRetirement {
	result := executionActivityRetirement{}
	for toolCallID, work := range ta.background {
		if work.executionID == executionID {
			delete(ta.background, toolCallID)
			result.removed = true
			result.removedSubagent =
				result.removedSubagent || work.kind == streams.BackgroundWorkKindSubagent
		}
	}
	for toolCallID, record := range ta.tools {
		if record.executionID == executionID {
			delete(ta.tools, toolCallID)
			result.removed = true
		}
	}
	result.removed = ta.retireExecutionClaimLocked(executionID) || result.removed

	result.finalTransition = ta.clearYieldWhenEmptyLocked()
	if result.removed || result.finalTransition {
		ta.revision++
	}

	hasSuccessorUse := ta.hasSuccessorUseLocked()
	ta.retireWhenUnused = !ta.hasExecutionOwnedActivityLocked() &&
		(ta.promptInFlight || ta.dispatchInFlightGeneration != 0)
	if !hasSuccessorUse {
		// Invalidate snapshots before detaching. The stable publication guard
		// makes a validated predecessor delivery finish before detachment, while
		// every later snapshot observes the missing record identity.
		ta.revision++
		result.retireRecord = true
	}
	return result
}

func (ta *turnActivity) hasSuccessorUseLocked() bool {
	if ta.promptInFlight || ta.dispatchInFlightGeneration != 0 {
		return true
	}
	return ta.hasExecutionOwnedActivityLocked()
}

func (ta *turnActivity) hasExecutionOwnedActivityLocked() bool {
	if len(ta.executionClaims) != 0 {
		return true
	}
	for _, work := range ta.background {
		if work.executionID != "" {
			return true
		}
	}
	for _, record := range ta.tools {
		if record.executionID != "" {
			return true
		}
	}
	return false
}

func (s *Service) pruneTurnActivityIfUnused(sessionID string, ta *turnActivity) {
	if sessionID == "" || ta == nil {
		return
	}
	publicationLock, releasePublication := s.acquireActivityPublicationGuard(sessionID)
	defer releasePublication()
	publicationLock.Lock()
	defer publicationLock.Unlock()
	s.foregroundActivityMu.Lock()
	current, ok := s.foregroundActivity[sessionID]
	if !ok || current != ta {
		s.foregroundActivityMu.Unlock()
		return
	}
	ta.mu.Lock()
	if ta.retireWhenUnused &&
		len(ta.executionClaims) == 0 &&
		!ta.promptInFlight &&
		ta.dispatchInFlightGeneration == 0 &&
		len(ta.background) == 0 &&
		len(ta.tools) == 0 {
		ta.revision++
		delete(s.foregroundActivity, sessionID)
	}
	ta.mu.Unlock()
	s.foregroundActivityMu.Unlock()
}

func (ta *turnActivity) clearYieldWhenEmptyLocked() bool {
	if len(ta.background) != 0 || !ta.yielded {
		return false
	}
	ta.yielded = false
	return true
}

func (s *Service) retireExecutionActivityAndPublish(
	ctx context.Context,
	taskID, sessionID, executionID string,
) {
	publication, changed := s.retireExecutionActivitySnapshot(sessionID, executionID)
	if changed {
		s.publishForegroundActivitySnapshot(ctx, taskID, sessionID, publication)
	}
}

// clearTurnActivity drops all tracked activity for a session. Foreground turn
// completion deliberately does not call it because detached work may outlive
// that turn; execution teardown and session removal do.
func (s *Service) clearTurnActivity(sessionID string) {
	if sessionID == "" {
		return
	}
	publicationLock, releasePublication := s.acquireActivityPublicationGuard(sessionID)
	defer releasePublication()
	publicationLock.Lock()
	defer publicationLock.Unlock()
	s.foregroundActivityMu.Lock()
	if ta, ok := s.foregroundActivity[sessionID]; ok {
		ta.mu.Lock()
		ta.revision++
		delete(s.foregroundActivity, sessionID)
		ta.mu.Unlock()
	}
	s.foregroundActivityMu.Unlock()
}

// isForegroundTurnGenerating reports whether the session's foreground agent turn
// is actively generating. It returns true (generating) unless the turn has
// yielded to an outstanding background task. An untracked session defaults to
// true, preserving the historical "reject a new prompt while RUNNING" contract.
//
// This is a pure internal predicate. The coarse default admission and
// serialization seams deliberately do not use it.
func (s *Service) isForegroundTurnGenerating(sessionID string) bool {
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return true
	}
	defer ta.mu.Unlock()
	return ta.generatingLocked()
}

// generatingLocked is the single definition of "the foreground turn is busy".
// An admitted-but-not-yet-dispatched prompt counts as busy in its own right: until
// it reaches the agent the session must not admit another, regardless of what the
// background set does underneath it.
func (ta *turnActivity) generatingLocked() bool {
	if ta.promptInFlight {
		return true
	}
	return !ta.yielded
}

// claimForegroundTurn is the retained check-and-claim mechanism for the private
// background-idle estimate.
// It atomically verifies the session's foreground turn has yielded to background
// work and, if so, takes it for the caller by flipping it back to generating —
// under the same lock, so exactly one caller can win.
//
// The public coarse gate rejects RUNNING sessions before this is reached unless
// the Claude-only experiment is enabled. Atomic claims preserve serialization
// for the experimental path.
//
// Returns nil for an untracked session — no background work is outstanding, so
// there is nothing to claim and the historical reject-while-RUNNING default
// stands — and for a session that already has a prompt in flight.
func (s *Service) claimForegroundTurn(sessionID string) *foregroundClaim {
	if sessionID == "" {
		return nil
	}
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		return nil
	}
	defer ta.mu.Unlock()
	if ta.generatingLocked() {
		return nil
	}
	ta.yielded = false
	ta.promptInFlight = true
	ta.claimGeneration++
	ta.revision++
	return &foregroundClaim{
		sessionID:       sessionID,
		activity:        ta,
		claimGeneration: ta.claimGeneration,
		foregroundEpoch: ta.foregroundEpoch,
	}
}

// isForegroundClaimCurrent reports whether claim still owns sessionID's prompt
// admission window. The guarded session-state recheck uses this to distinguish
// its own claim from a competing prompt that has already taken the foreground.
func (s *Service) isForegroundClaimCurrent(sessionID string, claim *foregroundClaim) bool {
	if sessionID == "" || claim == nil || claim.activity == nil {
		return false
	}
	if claim.sessionID != sessionID || !s.lockTurnActivityToken(sessionID, claim.activity) {
		return false
	}
	ta := claim.activity
	defer ta.mu.Unlock()
	return ta.promptInFlight && ta.claimGeneration == claim.claimGeneration
}

// beginForegroundDispatch atomically establishes both immutable prompt-cycle
// identity and foreground-generating ownership before calling agentctl. Keeping
// generation, yielded, and revision in one mutation prevents old cleanup from
// creating a newer null snapshot between "begin" and a later foreground flip.
// agentctl starts its prompt goroutine before returning the accepted response,
// so provider frames can also beat onDispatched.
func (s *Service) beginForegroundDispatch(
	sessionID string,
	claim *foregroundClaim,
	executionIDs ...string,
) *foregroundDispatch {
	executionID := ""
	if len(executionIDs) > 0 {
		executionID = executionIDs[0]
	}
	var ta *turnActivity
	if claim != nil {
		ta = claim.activity
		if ta == nil || claim.sessionID != sessionID ||
			!s.lockTurnActivityToken(sessionID, ta) {
			return nil
		}
	} else {
		if sessionID == "" {
			return nil
		}
		ta = s.lockTurnActivity(sessionID, true)
	}

	defer ta.mu.Unlock()
	if claim != nil {
		if !ta.promptInFlight || ta.claimGeneration != claim.claimGeneration {
			return nil
		}
	} else if ta.promptInFlight {
		// Fail closed: a live claimed admission is mid-dispatch (its accept has
		// not run yet). Resume can advance durable state to WAITING_FOR_INPUT
		// while that claimed prompt is still in flight (see
		// recheckPromptableWithForegroundClaim's resume comment in
		// task_operations.go), which lets a second, claimless prompt reach here.
		// Admitting it would bump promptCycleGeneration out from under the first
		// dispatch; its later acceptForegroundDispatch would then see a stale
		// generation and — before this fix — never clear promptInFlight, leaving
		// the session permanently reporting busy. Reject instead.
		return nil
	}
	yieldedBeforeBegin := ta.yielded
	ta.yielded = false
	ta.promptCycleGeneration++
	ta.claimExecutionLocked(executionID)
	ta.dispatchInFlightGeneration = ta.promptCycleGeneration
	// Starting any successor cycle invalidates delayed terminal/background
	// publications, including claimless prompts admitted from a settled state.
	ta.revision++
	return &foregroundDispatch{
		sessionID:             sessionID,
		activity:              ta,
		generation:            ta.promptCycleGeneration,
		foregroundEpoch:       ta.foregroundEpoch,
		claimGeneration:       ta.claimGeneration,
		claimedBackgroundTurn: claim != nil,
		yieldedBeforeBegin:    yieldedBeforeBegin,
	}
}

// acceptForegroundDispatch closes a retained background-idle claim after
// agentctl acknowledges the prompt. The cycle itself was already established by
// beginForegroundDispatch, so pre-callback provider events own the right token.
func (s *Service) acceptForegroundDispatch(dispatch *foregroundDispatch) bool {
	if dispatch == nil || dispatch.activity == nil {
		return false
	}
	ta := dispatch.activity
	if !s.lockTurnActivityToken(dispatch.sessionID, ta) {
		return false
	}
	defer func() {
		ta.mu.Unlock()
		s.pruneTurnActivityIfUnused(dispatch.sessionID, ta)
	}()
	if ta.dispatchInFlightGeneration == dispatch.generation {
		ta.dispatchInFlightGeneration = 0
	}

	// Release the claimed admission before the cycle-generation check below:
	// resume can advance durable state to WAITING_FOR_INPUT while this claimed
	// prompt is still mid-dispatch (see recheckPromptableWithForegroundClaim's
	// resume comment in task_operations.go), letting a second, claimless prompt
	// be admitted and bump promptCycleGeneration out from under this dispatch.
	// Gating the release on `ta.promptCycleGeneration == dispatch.generation`
	// would then never fire, stranding promptInFlight true forever and making
	// generatingLocked() report busy for the rest of the session's life.
	if dispatch.claimedBackgroundTurn && ta.promptInFlight && ta.claimGeneration == dispatch.claimGeneration {
		ta.promptInFlight = false
	}

	if dispatch.accepted || ta.promptCycleGeneration != dispatch.generation {
		return false
	}
	dispatch.accepted = true
	if !dispatch.claimedBackgroundTurn {
		return false
	}
	if ta.yielded {
		return true
	}
	completedCurrent := ta.hasCompletedGeneration && ta.completedGeneration == dispatch.generation
	idleCurrentWithoutLaterOutput := ta.hasPendingCompletionGeneration &&
		ta.pendingCompletionGeneration == dispatch.generation &&
		ta.foregroundEpoch == dispatch.foregroundEpoch
	if completedCurrent || idleCurrentWithoutLaterOutput {
		return ta.markForegroundIdleLocked()
	}
	return false
}

func (s *Service) acceptedForegroundDispatchClaim(dispatch *foregroundDispatch) bool {
	if dispatch == nil || dispatch.activity == nil {
		return false
	}
	ta := dispatch.activity
	if !s.lockTurnActivityToken(dispatch.sessionID, ta) {
		return false
	}
	defer ta.mu.Unlock()
	return dispatch.accepted && dispatch.claimedBackgroundTurn
}

// rollbackForegroundDispatch releases a failed, unobserved admission. Prompt
// generations are never reused: leaving the failed generation consumed makes a
// stale rollback distinguishable after a retry establishes its successor.
// Generation, epoch, pending-completion, and completed-generation checks make
// the rollback token-aware, so failure cannot clobber provider work that arrived.
func (s *Service) rollbackForegroundDispatch(dispatch *foregroundDispatch) bool {
	if dispatch == nil || dispatch.activity == nil {
		return false
	}
	ta := dispatch.activity
	if !s.lockTurnActivityToken(dispatch.sessionID, ta) {
		return false
	}
	defer func() {
		ta.mu.Unlock()
		s.pruneTurnActivityIfUnused(dispatch.sessionID, ta)
	}()
	if ta.dispatchInFlightGeneration == dispatch.generation {
		ta.dispatchInFlightGeneration = 0
	}
	// Release the claimed admission unconditionally, before the
	// dispatchRollbackIsCurrentLocked staleness check: that check gates on
	// promptCycleGeneration/foregroundEpoch, which a claimless successor
	// prompt's beginForegroundDispatch can advance out from under this
	// dispatch while the admission claim (claimGeneration) is still this
	// dispatch's own. Short-circuiting the release on that unrelated
	// staleness check would strand promptInFlight — see
	// acceptForegroundDispatch's comment for the same failure mode.
	released := ta.releaseDispatchClaimLocked(dispatch)
	if !ta.dispatchRollbackIsCurrentLocked(dispatch) || !released {
		return false
	}
	restoreBackground := dispatch.claimedBackgroundTurn || dispatch.yieldedBeforeBegin
	if len(ta.background) == 0 || !restoreBackground {
		return false
	}
	ta.yielded = true
	ta.revision++
	return true
}

func (ta *turnActivity) dispatchRollbackIsCurrentLocked(dispatch *foregroundDispatch) bool {
	if dispatch.accepted || ta.promptCycleGeneration != dispatch.generation ||
		ta.foregroundEpoch != dispatch.foregroundEpoch {
		return false
	}
	if ta.hasPendingCompletionGeneration && ta.pendingCompletionGeneration == dispatch.generation {
		return false
	}
	return !ta.hasCompletedGeneration || ta.completedGeneration != dispatch.generation
}

func (ta *turnActivity) releaseDispatchClaimLocked(dispatch *foregroundDispatch) bool {
	if !dispatch.claimedBackgroundTurn {
		return true
	}
	if !ta.promptInFlight || ta.claimGeneration != dispatch.claimGeneration {
		return false
	}
	ta.promptInFlight = false
	return true
}

// releaseForegroundClaim hands a claimForegroundTurn claim back when the prompt
// it was taken for never made it to the agent (ensureSessionRunning failed, the
// model switch failed). Without it the session would sit in RUNNING advertising a
// generating foreground it does not have, locking the operator out for the rest
// of the turn — the exact lockout ADR-0049 exists to remove.
//
// It reports whether the turn was actually handed back to background-idle, so the
// caller can broadcast the restored substate. Two things stop a release from
// opening the gate when it shouldn't:
//
//   - The epoch. If the agent's foreground streamed real output while this prompt
//     was in preflight, markForegroundGenerating bumped the epoch: the turn is
//     genuinely generating now, and handing it back to background-idle would let a
//     second prompt overlap a live turn.
//   - The background set. If the last background task finished while the failing
//     prompt was in flight, nothing is outstanding and the generating default is
//     correct.
func (s *Service) releaseForegroundClaim(claim *foregroundClaim) bool {
	if claim == nil || claim.activity == nil {
		return false
	}
	ta := claim.activity
	if !s.lockTurnActivityToken(claim.sessionID, ta) {
		return false
	}
	if !ta.promptInFlight || ta.claimGeneration != claim.claimGeneration {
		ta.mu.Unlock()
		return false
	}
	ta.promptInFlight = false
	if ta.foregroundEpoch != claim.foregroundEpoch || len(ta.background) == 0 {
		ta.mu.Unlock()
		s.pruneTurnActivityIfUnused(claim.sessionID, ta)
		return false
	}
	ta.yielded = true
	ta.mu.Unlock()
	return true
}

// foregroundActivityValue reports the best-effort internal activity estimate.
// ACP providers do not expose background work with consistent enough lifecycle
// semantics for this estimate to control operator-facing admission or busy
// state. Keep it available for dormant tracking and diagnostics only.
func (s *Service) foregroundActivityValue(sessionID string) v1.ForegroundActivity {
	if s.isForegroundTurnGenerating(sessionID) {
		return v1.ForegroundActivityGenerating
	}
	return v1.ForegroundActivityBackground
}

const (
	claudeACPProviderID = "claude-acp"
	mockAgentProviderID = "mock-agent"
)

// claudeBackgroundPromptHandoffEnabled reports whether sessionID is allowed to
// use the high-risk fine-grained policy. Provider identity comes from the
// persisted launch snapshot rather than a client-supplied activity value. A
// missing session, snapshot, or provider fails closed.
func (s *Service) claudeBackgroundPromptHandoffEnabled(sessionID string) bool {
	if !s.config.ClaudeBackgroundPromptHandoff || s.repo == nil || sessionID == "" {
		return false
	}
	session, err := s.repo.GetTaskSession(context.Background(), sessionID)
	if err != nil || session == nil {
		return false
	}
	return s.claudeBackgroundPromptHandoffEnabledForSession(session)
}

func (s *Service) claudeBackgroundPromptHandoffEnabledForSession(
	session *models.TaskSession,
) bool {
	if !s.config.ClaudeBackgroundPromptHandoff || session == nil {
		return false
	}
	agentName, _ := session.AgentProfileSnapshot["agent_name"].(string)
	return agentName == claudeACPProviderID || agentName == mockAgentProviderID
}

// ForegroundActivity exposes the conservative operator-facing busy signal by
// default. A deployment that explicitly enables the experiment may expose the
// private tracker value only for a persisted Claude Code session (or the mock
// provider used to exercise Claude lifecycle frames in E2E).
func (s *Service) ForegroundActivity(sessionID string) v1.ForegroundActivity {
	if s.claudeBackgroundPromptHandoffEnabled(sessionID) {
		return s.foregroundActivityValue(sessionID)
	}
	return v1.ForegroundActivityGenerating
}

// publishForegroundActivityChanged emits a provider-aware refresh when the
// internal tracker changes. publishForegroundActivityNow applies the coarse
// default or the explicitly enabled Claude experiment before broadcasting.
func (s *Service) publishForegroundActivityChanged(ctx context.Context, taskID, sessionID string) {
	publicationLock, releasePublication := s.acquireActivityPublicationGuard(sessionID)
	defer releasePublication()
	publicationLock.Lock()
	defer publicationLock.Unlock()
	ta := s.lockTurnActivity(sessionID, false)
	if ta == nil {
		s.publishForegroundActivityNow(
			ctx, taskID, sessionID, string(v1.ForegroundActivityGenerating), 0,
		)
		return
	}
	value := v1.ForegroundActivityGenerating
	if !ta.generatingLocked() {
		value = v1.ForegroundActivityBackground
	}
	activeSubagentCount := ta.activeSubagentCountLocked()
	ta.mu.Unlock()
	s.publishForegroundActivityNow(ctx, taskID, sessionID, string(value), activeSubagentCount)
}

func (s *Service) publishForegroundActivitySnapshot(
	ctx context.Context,
	taskID, sessionID string,
	publication activityPublication,
) {
	ta := publication.activity
	if ta == nil {
		return
	}
	publicationLock, releasePublication := s.acquireActivityPublicationGuard(sessionID)
	defer releasePublication()
	publicationLock.Lock()
	defer publicationLock.Unlock()
	current := false
	if publication.retired {
		s.foregroundActivityMu.Lock()
		mapped, ok := s.foregroundActivity[sessionID]
		if !ok || mapped == ta {
			ta.mu.Lock()
			current = !ok && ta.revision == publication.revision
			ta.mu.Unlock()
		}
		s.foregroundActivityMu.Unlock()
	} else if s.lockTurnActivityToken(sessionID, ta) {
		current = ta.revision == publication.revision
		ta.mu.Unlock()
	}
	if !current {
		return
	}
	s.publishForegroundActivityNow(
		ctx,
		taskID,
		sessionID,
		publication.value,
		publication.activeSubagentCount,
	)
}

func (s *Service) publishForegroundActivityNow(
	ctx context.Context,
	taskID, sessionID string,
	value interface{},
	activeSubagentCount int,
) {
	if s.eventBus == nil || taskID == "" || sessionID == "" {
		return
	}
	eventData := map[string]interface{}{
		metaKeyTaskID:           taskID,
		metaKeySessionID:        sessionID,
		"foreground_activity":   s.publicForegroundActivityValue(ctx, sessionID, value),
		"active_subagent_count": activeSubagentCount,
	}
	if err := s.eventBus.Publish(ctx, events.TaskSessionActivityChanged,
		bus.NewEvent(events.TaskSessionActivityChanged, "task-session", eventData)); err != nil {
		s.logger.Warn("publish task_session.activity_changed failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	// Propagate the flip to the task-level aggregate so at-a-glance task surfaces
	// (board card, task list) update live; emits task.updated only when the
	// task-level three-state value actually changes.
	s.publishTaskActivityIfChanged(ctx, taskID)
}

func (s *Service) publicForegroundActivityValue(
	ctx context.Context,
	sessionID string,
	value interface{},
) interface{} {
	if value == nil {
		return nil
	}
	publicValue := interface{}(string(v1.ForegroundActivityGenerating))
	if s.repo == nil {
		return publicValue
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return publicValue
	}
	if s.claudeBackgroundPromptHandoffEnabledForSession(session) &&
		value == string(v1.ForegroundActivityBackground) {
		return value
	}
	if session.State != models.TaskSessionStateRunning {
		return nil
	}
	return publicValue
}

// normalizedIsBackgroundTask reports whether the adapter attested a live
// workload whose complete lifecycle Kandev recognizes.
//
// A create_task tool is deliberately NOT background — it spawns an independent
// task/session with its own lifecycle and does not hold the spawning turn open.
func normalizedIsBackgroundTask(n *streams.NormalizedPayload) bool {
	return n != nil && n.IsActiveBackgroundWork()
}

// normalizedIsDetachedLaunch distinguishes a launch tool completing from its
// asynchronously-running workload completing.
func normalizedIsDetachedLaunch(n *streams.NormalizedPayload) bool {
	return n != nil && n.IsDetachedBackgroundLaunch()
}
