package slack

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultBaseInterval is how often the trigger's base loop wakes up to check
// whether a scan is due. The actual scan cadence comes from
// SlackConfig.PollIntervalSeconds; this just bounds how soon a freshly-due
// scan gets noticed.
const DefaultBaseInterval = 5 * time.Second

// AcknowledgeReaction is the emoji we react with on a matched message before
// the agent runs.
const AcknowledgeReaction = "eyes"

// Trigger drives the polling loop that turns matched Slack messages into
// utility-agent runs. The loop is install-wide singleton — there's at most
// one Slack-search-per-PollIntervalSeconds running, regardless of how many
// Kandev workspaces exist.
type Trigger struct {
	svc      *Service
	log      *logger.Logger
	interval time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool

	// matchHook fires after each successfully-handled match. Tests use it
	// to synchronise on processing without sleep-polling.
	matchHook func(msg SlackMessage, reply string)

	// lastScannedAt tracks when we last ran a scan so the configured
	// PollIntervalSeconds is honoured exactly. In-memory by design — backend
	// restart triggers an immediate scan, which is fine.
	scannedMu     sync.Mutex
	lastScannedAt time.Time
}

// NewTrigger returns a Trigger using the default base ticker.
func NewTrigger(svc *Service, log *logger.Logger) *Trigger {
	if svc == nil {
		return nil
	}
	return &Trigger{svc: svc, log: log, interval: DefaultBaseInterval}
}

// SetInterval overrides the base-ticker cadence. Must be called before Start.
func (t *Trigger) SetInterval(d time.Duration) {
	t.mu.Lock()
	t.interval = d
	t.mu.Unlock()
}

// SetMatchHook installs a callback fired after each successfully-handled
// match.
func (t *Trigger) SetMatchHook(fn func(msg SlackMessage, reply string)) {
	t.mu.Lock()
	t.matchHook = fn
	t.mu.Unlock()
}

// Start launches the loop.
func (t *Trigger) Start(ctx context.Context) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return
	}
	t.started = true
	ctx, t.cancel = context.WithCancel(ctx)
	t.wg.Add(1)
	go t.loop(ctx)
	t.log.Info("slack trigger started")
}

// Stop cancels the loop and waits for it to drain.
func (t *Trigger) Stop() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if !t.started {
		t.mu.Unlock()
		return
	}
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	t.wg.Wait()
	t.mu.Lock()
	t.started = false
	t.mu.Unlock()
	t.log.Info("slack trigger stopped")
}

func (t *Trigger) loop(ctx context.Context) {
	defer t.wg.Done()
	t.tick(ctx)
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.tick(ctx)
		}
	}
}

// tick scans Slack once if the configured cadence has elapsed.
func (t *Trigger) tick(ctx context.Context) {
	if t.svc.Runner() == nil {
		return
	}
	cfg, err := t.svc.Store().GetConfig(ctx)
	if err != nil || cfg == nil {
		return
	}
	if !cfg.LastOk || cfg.SlackUserID == "" {
		return
	}
	if cfg.UtilityAgentID == "" {
		return
	}
	if !t.dueForScan(cfg.PollIntervalSeconds) {
		return
	}
	prefix := cfg.CommandPrefix
	if prefix == "" {
		prefix = DefaultCommandPrefix
	}
	client, err := t.svc.Client(ctx)
	if err != nil {
		return
	}
	t.markScanned()
	query := fmt.Sprintf("from:<@%s> %q", cfg.SlackUserID, prefix)
	matches, err := client.SearchMessages(ctx, query)
	if err != nil {
		t.log.Warn("slack trigger: search failed", zap.Error(err))
		return
	}
	t.processMatches(ctx, cfg, prefix, matches, client)
}

func (t *Trigger) processMatches(ctx context.Context, cfg *SlackConfig, prefix string, matches []SlackMessage, client Client) {
	fresh := newMatchesAfter(matches, cfg.LastSeenTS, prefix)
	if len(fresh) == 0 {
		return
	}
	highest := cfg.LastSeenTS
	for _, m := range fresh {
		if ctx.Err() != nil {
			return
		}
		reply, err := t.handleOne(ctx, cfg, prefix, m, client)
		if err != nil {
			t.log.Warn("slack trigger: handle match failed",
				zap.String("ts", m.TS), zap.Error(err))
			if errors.Is(err, ErrNoUtilityAgent) {
				if compareTS(m.TS, highest) > 0 {
					highest = m.TS
				}
				continue
			}
			// Recoverable failure: stop the loop here so the watermark stays
			// at the last consecutive success. See the original commit for
			// why this is a `break` (not `return`) — the post-loop
			// UpdateLastSeenTS still needs to persist prior successes.
			break
		}
		if compareTS(m.TS, highest) > 0 {
			highest = m.TS
		}
		t.fireHook(m, reply)
	}
	if highest != cfg.LastSeenTS {
		if err := t.svc.Store().UpdateLastSeenTS(ctx, highest); err != nil {
			t.log.Warn("slack trigger: advance watermark failed", zap.Error(err))
		}
	}
}

// handleOne runs the per-match flow: ack reaction, fetch thread, run the
// utility agent, post the reply in-thread.
func (t *Trigger) handleOne(ctx context.Context, cfg *SlackConfig, prefix string, msg SlackMessage, client Client) (string, error) {
	if err := client.ReactionsAdd(ctx, msg.ChannelID, msg.TS, AcknowledgeReaction); err != nil {
		t.log.Warn("slack trigger: ack reaction failed",
			zap.String("ts", msg.TS), zap.Error(err))
	}

	thread, err := client.ConversationsReplies(ctx, msg.ChannelID, msg.ThreadTS, msg.TS)
	if err != nil {
		return "", fmt.Errorf("fetch thread: %w", err)
	}
	permalink := msg.Permalink
	if permalink == "" {
		permalink, _ = client.ChatGetPermalink(ctx, msg.ChannelID, msg.TS)
	}
	instruction := stripPrefix(msg.Text, prefix)

	runner := t.svc.Runner()
	if runner == nil {
		return "", errors.New("agent runner not configured")
	}
	reply, err := runner.RunForMatch(ctx, cfg, msg, instruction, permalink, thread)
	if err != nil {
		return "", err
	}
	t.replyInThread(ctx, client, msg, reply)
	return reply, nil
}

func (t *Trigger) replyInThread(ctx context.Context, client Client, msg SlackMessage, body string) {
	threadTS := msg.ThreadTS
	if threadTS == "" {
		threadTS = msg.TS
	}
	if err := client.ChatPostMessage(ctx, msg.ChannelID, threadTS, body); err != nil {
		t.log.Warn("slack trigger: reply post failed",
			zap.String("ts", msg.TS), zap.Error(err))
	}
}

func (t *Trigger) fireHook(msg SlackMessage, reply string) {
	t.mu.Lock()
	hook := t.matchHook
	t.mu.Unlock()
	if hook != nil {
		hook(msg, reply)
	}
}

// dueForScan reports whether the configured interval has elapsed since the
// last scan. A zero/never timestamp counts as due.
func (t *Trigger) dueForScan(intervalSeconds int) bool {
	if intervalSeconds < MinPollIntervalSeconds {
		intervalSeconds = DefaultPollIntervalSeconds
	}
	t.scannedMu.Lock()
	defer t.scannedMu.Unlock()
	if t.lastScannedAt.IsZero() {
		return true
	}
	return time.Since(t.lastScannedAt) >= time.Duration(intervalSeconds)*time.Second
}

func (t *Trigger) markScanned() {
	t.scannedMu.Lock()
	t.lastScannedAt = time.Now()
	t.scannedMu.Unlock()
}

func newMatchesAfter(matches []SlackMessage, watermark, prefix string) []SlackMessage {
	out := make([]SlackMessage, 0, len(matches))
	for _, m := range matches {
		if compareTS(m.TS, watermark) <= 0 {
			continue
		}
		if !startsWithPrefix(m.Text, prefix) {
			continue
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return compareTS(out[i].TS, out[j].TS) < 0
	})
	return out
}

func startsWithPrefix(text, prefix string) bool {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "> ")
	t = strings.TrimSpace(t)
	if !strings.HasPrefix(strings.ToLower(t), strings.ToLower(prefix)) {
		return false
	}
	rest := t[len(prefix):]
	if rest == "" {
		return true
	}
	r := rest[0]
	return r == ' ' || r == '\t' || r == '\n' || r == ':' || r == ','
}

func stripPrefix(text, prefix string) string {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "> ")
	t = strings.TrimSpace(t)
	if len(t) >= len(prefix) && strings.EqualFold(t[:len(prefix)], prefix) {
		t = t[len(prefix):]
	}
	t = strings.TrimSpace(t)
	t = strings.TrimLeft(t, ":, ")
	return strings.TrimSpace(t)
}

func compareTS(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	if a < b {
		return -1
	}
	return 1
}
