package acp

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// wakeupScheduler holds at most one pending ScheduleWakeup-driven prompt for
// the adapter. The Claude Agent SDK exposes `ScheduleWakeup` as a tool inside
// its harness; when its timer fires, the SDK queues a new turn on its
// async-iterator (`session.query`). The upstream
// @agentclientprotocol/claude-agent-acp bridge only iterates that channel
// inside its `prompt()` handler, so a wakeup that fires while no
// `session/prompt` is in flight produces no ACP output at all — the buffered
// turn only drains on the next user message.
//
// This scheduler closes the gap: when we observe a `ScheduleWakeup` tool
// completion, we record `scheduledFor` and the prompt text, and at fire time
// issue a synthetic `session/prompt` to the bridge. That triggers the bridge
// to drain the buffered output and produce visible ACP frames for the wakeup
// turn — matching what standalone Claude Code does, where the wakeup prompt
// is injected as a synthesized user turn.
type wakeupScheduler struct {
	logger *logger.Logger
	fire   func(sessionID, prompt string)

	mu        sync.Mutex
	timer     *time.Timer
	sessionID string
	prompt    string
	// gen is incremented on every schedule/cancel. fireOnce captures the gen
	// at scheduling time and refuses to fire if it doesn't match the current
	// gen — that closes the race where time.AfterFunc has already launched a
	// goroutine for the old timer (timer.Stop() returns false) and the new
	// schedule() then writes new state before the stale fireOnce acquires the
	// lock. Without gen, the stale fireOnce would consume the new state and
	// fire the new wakeup immediately while clobbering w.timer.
	gen uint64
}

// pendingWakeup accumulates partial state for an in-flight ScheduleWakeup tool
// call. The bridge emits the wakeup metadata across multiple notifications:
// the initial tool_call carries `_meta.claudeCode.toolName="ScheduleWakeup"`
// (no rawInput yet); subsequent tool_call_updates fill in `rawInput.prompt`
// and `_meta.claudeCode.toolResponse.scheduledFor`. We only have enough to
// schedule once both the prompt and scheduledFor timestamp are present.
type pendingWakeup struct {
	prompt         string
	scheduledForMs int64
}

func newWakeupScheduler(log *logger.Logger, fire func(sessionID, prompt string)) *wakeupScheduler {
	return &wakeupScheduler{
		logger: log.WithFields(zap.String("component", "wakeup-scheduler")),
		fire:   fire,
	}
}

// schedule registers a wakeup for the given session, replacing any prior
// pending wakeup. unixMs is the Unix-epoch millisecond timestamp at which the
// wakeup should fire (matches the `_meta.claudeCode.toolResponse.scheduledFor`
// field reported by the bridge). If the timestamp is in the past or zero, the
// wakeup is dropped without firing — that matches the ACP semantics of "the
// turn already happened" and avoids unbounded immediate-fire loops on stale
// timestamps.
func (w *wakeupScheduler) schedule(sessionID, prompt string, unixMs int64) {
	if sessionID == "" || prompt == "" || unixMs <= 0 {
		w.logger.Warn("ignoring wakeup with missing fields",
			zap.String("session_id", sessionID),
			zap.Int("prompt_len", len(prompt)),
			zap.Int64("unix_ms", unixMs))
		return
	}

	delay := time.Until(time.UnixMilli(unixMs))
	if delay <= 0 {
		// Warn rather than Debug: a stale wakeup typically signals system
		// suspension, container pause, or significant clock skew — those are
		// production-relevant and shouldn't be hidden at default log levels.
		w.logger.Warn("dropping stale wakeup",
			zap.String("session_id", sessionID),
			zap.Duration("past_by", -delay))
		return
	}

	w.mu.Lock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.gen++
	myGen := w.gen
	w.sessionID = sessionID
	w.prompt = prompt
	w.timer = time.AfterFunc(delay, func() { w.fireOnce(myGen) })
	w.mu.Unlock()

	w.logger.Info("scheduled wakeup",
		zap.String("session_id", sessionID),
		zap.Time("fires_at", time.UnixMilli(unixMs)),
		zap.Duration("delay", delay))
}

// fireOnce is invoked by the timer and dispatches to the configured callback.
// myGen identifies the schedule call that armed this timer; if a later
// schedule or cancel has bumped w.gen, this fire is stale and is dropped.
// It clears the pending state before calling the callback so a re-entry from
// inside the callback (e.g. the synthetic prompt schedules another wakeup)
// can overwrite cleanly. The gen check guarantees sessionID is non-empty
// here — schedule() rejects empty sessionID before bumping gen, and any
// cancel would have bumped gen too — so we don't re-check it.
func (w *wakeupScheduler) fireOnce(myGen uint64) {
	w.mu.Lock()
	if myGen != w.gen {
		w.mu.Unlock()
		return
	}
	sessionID := w.sessionID
	prompt := w.prompt
	w.timer = nil
	w.sessionID = ""
	w.prompt = ""
	w.gen++
	w.mu.Unlock()

	w.logger.Info("firing wakeup",
		zap.String("session_id", sessionID))
	w.fire(sessionID, prompt)
}

// cancel stops any pending wakeup. Safe to call multiple times. Bumping gen
// invalidates any in-flight fireOnce that has already been launched but is
// blocked on the lock — without that, a stale fireOnce after Stop()=false
// would still observe the cleared state and no-op cleanly, but cancel races
// with concurrent schedule could otherwise let a stale fire consume newly
// scheduled state.
func (w *wakeupScheduler) cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.sessionID = ""
	w.prompt = ""
	w.gen++
}

// extractScheduleWakeup inspects an ACP tool-call meta map and returns
// (scheduledForUnixMs, isScheduleWakeup). It expects the bridge's
// `_meta.claudeCode.toolName` and `_meta.claudeCode.toolResponse.scheduledFor`
// shape that @agentclientprotocol/claude-agent-acp emits; missing/malformed
// entries are reported as not-a-wakeup rather than as errors so unrelated
// tools pass through unaffected.
func extractScheduleWakeup(meta any) (scheduledForMs int64, isWakeup bool) {
	m, ok := meta.(map[string]any)
	if !ok {
		return 0, false
	}
	cc, ok := m["claudeCode"].(map[string]any)
	if !ok {
		return 0, false
	}
	if name, _ := cc["toolName"].(string); name != "ScheduleWakeup" {
		return 0, false
	}
	resp, ok := cc["toolResponse"].(map[string]any)
	if !ok {
		return 0, true // ScheduleWakeup tool, but response not yet present
	}
	switch v := resp["scheduledFor"].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, true
}

// extractWakeupPrompt pulls the prompt string out of a ScheduleWakeup
// tool-call rawInput. Returns ("", false) when the field is absent or the
// wrong type.
func extractWakeupPrompt(rawInput any) (string, bool) {
	m, ok := rawInput.(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := m["prompt"].(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}
