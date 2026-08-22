package hostnames

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

const (
	lookupWorkers       = 4
	lookupTimeout       = 3 * time.Second
	recentResolveWindow = 30 * time.Second
	userIDKey           = "user_id"
)

type SettingsGate func(context.Context, string) (bool, error)
type SessionIPs func(context.Context, string) ([]string, error)
type LookupAddr func(context.Context, string) ([]string, error)

type interest struct {
	userID string
	rawIP  string
}

type pendingJob struct {
	ip         string
	observed   CacheEntry
	interests  []interest
	generation uint64
}

type pendingUser struct {
	generation uint64
	running    bool
}

type interestGate struct {
	generation uint64
	enabled    map[string]bool
}

func (g *interestGate) forGeneration(generation uint64) map[string]bool {
	if g.enabled == nil || g.generation != generation {
		g.generation = generation
		g.enabled = make(map[string]bool)
	}
	return g.enabled
}

// Resolver asynchronously reverse-resolves opted-in user session IPs.
type Resolver struct {
	cache      Cache
	settingsOn SettingsGate
	sessionIPs SessionIPs
	lookupAddr LookupAddr
	eventBus   bus.EventBus
	log        *logger.Logger

	mu           sync.Mutex
	pending      map[string]*pendingJob
	inFlight     map[string]*pendingJob
	pendingUsers map[string]pendingUser
	wake         chan struct{}
	triggerWake  chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	started      bool
	stopping     chan struct{}
}

// NewResolver builds a resolver with independent lookup and trigger queues.
func NewResolver(cache Cache, settingsOn SettingsGate, sessionIPs SessionIPs, eventBus bus.EventBus, log *logger.Logger) *Resolver {
	return &Resolver{
		cache: cache, settingsOn: settingsOn, sessionIPs: sessionIPs, eventBus: eventBus, log: log,
		lookupAddr: net.DefaultResolver.LookupAddr,
		pending:    make(map[string]*pendingJob), inFlight: make(map[string]*pendingJob),
		pendingUsers: make(map[string]pendingUser), wake: make(chan struct{}, lookupWorkers), triggerWake: make(chan struct{}, 1),
	}
}

// Start launches lookup workers unless an earlier lifecycle is still closing.
func (r *Resolver) Start(parent context.Context) {
	for {
		r.mu.Lock()
		if r.started {
			r.mu.Unlock()
			return
		}
		if r.stopping != nil {
			stopping := r.stopping
			r.mu.Unlock()
			<-stopping
			continue
		}
		r.ctx, r.cancel = context.WithCancel(parent)
		r.started = true
		for range lookupWorkers {
			r.wg.Add(1)
			go r.worker()
		}
		r.wg.Add(1)
		go r.triggerWorker()
		r.mu.Unlock()
		return
	}
}

// Close cancels workers and resets queue state for a future Start.
func (r *Resolver) Close() error {
	r.mu.Lock()
	if r.stopping != nil {
		stopping := r.stopping
		r.mu.Unlock()
		<-stopping
		return nil
	}
	if !r.started {
		r.mu.Unlock()
		return nil
	}
	r.cancel()
	r.started = false
	r.stopping = make(chan struct{})
	stopping := r.stopping
	r.mu.Unlock()
	r.wg.Wait()

	r.mu.Lock()
	r.pending = make(map[string]*pendingJob)
	r.inFlight = make(map[string]*pendingJob)
	r.wake = make(chan struct{}, lookupWorkers)
	r.triggerWake = make(chan struct{}, 1)
	r.pendingUsers = make(map[string]pendingUser)
	r.stopping = nil
	close(stopping)
	r.mu.Unlock()
	return nil
}

// HostnamesForSessionIPs returns raw-IP keyed cache entries and admits stale IPs for lookup.
func (r *Resolver) HostnamesForSessionIPs(ctx context.Context, userID string, ips []string) map[string]CacheEntry {
	if r == nil || r.cache == nil || r.settingsOn == nil {
		return map[string]CacheEntry{}
	}
	enabled, err := r.settingsOn(ctx, userID)
	if err != nil || !enabled {
		r.logError("hostname settings gate", err)
		return map[string]CacheEntry{}
	}
	canonical, rawByCanonical := canonicalIPs(ips)
	if len(canonical) == 0 {
		return map[string]CacheEntry{}
	}
	cached, err := r.cache.GetMany(ctx, canonical)
	if err != nil {
		r.logError("read hostname cache", err)
		return map[string]CacheEntry{}
	}
	result := make(map[string]CacheEntry, len(ips))
	for canonicalIP, rawIPs := range rawByCanonical {
		entry, exists := cached[canonicalIP]
		for _, raw := range rawIPs {
			result[raw] = entry
		}
		if !exists || entry.ResolvedAt == nil || time.Since(*entry.ResolvedAt) >= recentResolveWindow {
			r.enqueue(canonicalIP, entry, userID, rawIPs)
		}
	}
	return result
}

// TriggerUserSessionsResolved coalesces a background lookup pass after settings are persisted.
func (r *Resolver) TriggerUserSessionsResolved(userID string) {
	if r == nil || r.sessionIPs == nil {
		return
	}
	r.mu.Lock()
	if !r.started || r.ctx == nil {
		r.mu.Unlock()
		return
	}
	pending := r.pendingUsers[userID]
	pending.generation++
	r.pendingUsers[userID] = pending
	triggerWake := r.triggerWake
	r.mu.Unlock()
	select {
	case triggerWake <- struct{}{}:
	default:
	}
}

// HandleUserSettingsUpdated starts a session pass after hostname resolution is enabled.
func (r *Resolver) HandleUserSettingsUpdated(_ context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	data, ok := event.Data.(map[string]any)
	if !ok {
		return nil
	}
	enabled, _ := data["resolve_session_hostnames"].(bool)
	userID, _ := data[userIDKey].(string)
	if enabled && userID != "" {
		r.TriggerUserSessionsResolved(userID)
	}
	return nil
}

// triggerWorker consumes coalesced user-session resolution requests.
func (r *Resolver) triggerWorker() {
	defer r.wg.Done()
	for {
		userID, generation := r.nextPendingUser()
		if userID != "" {
			ips, err := r.sessionIPs(r.ctx, userID)
			if err != nil {
				r.logError("list session IPs", err)
			} else {
				r.HostnamesForSessionIPs(r.ctx, userID, ips)
			}
			r.finishPendingUser(userID, generation)
			continue
		}
		select {
		case <-r.ctx.Done():
			return
		case <-r.triggerWake:
		}
	}
}

// nextPendingUser claims one queued user-session request.
func (r *Resolver) nextPendingUser() (string, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for userID, pending := range r.pendingUsers {
		if pending.running {
			continue
		}
		pending.running = true
		r.pendingUsers[userID] = pending
		return userID, pending.generation
	}
	return "", 0
}

// finishPendingUser removes a completed request unless newer work arrived.
func (r *Resolver) finishPendingUser(userID string, generation uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, exists := r.pendingUsers[userID]
	if !exists {
		return
	}
	if pending.generation != generation {
		pending.running = false
		r.pendingUsers[userID] = pending
		return
	}
	delete(r.pendingUsers, userID)
}

// enqueue coalesces an IP lookup and wakes an available worker.
func (r *Resolver) enqueue(ip string, observed CacheEntry, userID string, rawIPs []string) {
	r.mu.Lock()
	job := r.inFlight[ip]
	queued := false
	if job == nil {
		job = r.pending[ip]
		if job == nil {
			job = &pendingJob{ip: ip, observed: observed}
			r.pending[ip] = job
			queued = true
		}
	}
	job.generation++
	for _, rawIP := range rawIPs {
		exists := false
		for _, existing := range job.interests {
			if existing.userID == userID && existing.rawIP == rawIP {
				exists = true
				break
			}
		}
		if !exists {
			job.interests = append(job.interests, interest{userID: userID, rawIP: rawIP})
		}
	}
	wake := r.wake
	r.mu.Unlock()
	if !queued {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

// worker resolves queued IPs until cancellation.
func (r *Resolver) worker() {
	defer r.wg.Done()
	for {
		job := r.nextPending()
		if job == nil {
			select {
			case <-r.ctx.Done():
				return
			case <-r.wake:
			}
			continue
		}
		r.resolve(job)
	}
}

// nextPending claims one queued IP lookup.
func (r *Resolver) nextPending() *pendingJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ip, job := range r.pending {
		delete(r.pending, ip)
		r.inFlight[ip] = job
		return job
	}
	return nil
}

// finish removes a completed lookup and returns its interested sessions.
func (r *Resolver) finish(job *pendingJob) []interest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inFlight[job.ip] != job {
		return nil
	}
	delete(r.inFlight, job.ip)
	return append([]interest(nil), job.interests...)
}

// finishIfInterestsUnchanged removes a lookup only when no newer interest arrived.
func (r *Resolver) finishIfInterestsUnchanged(job *pendingJob, generation uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inFlight[job.ip] != job {
		return true
	}
	if job.generation != generation {
		return false
	}
	delete(r.inFlight, job.ip)
	return true
}

// snapshotInterests copies the current interested sessions for a lookup.
func (r *Resolver) snapshotInterests(job *pendingJob) ([]interest, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inFlight[job.ip] != job {
		return nil, 0
	}
	return append([]interest(nil), job.interests...), job.generation
}

// resolve loads or performs one reverse-DNS lookup and publishes changes.
func (r *Resolver) resolve(job *pendingJob) {
	current, err := r.cache.Get(r.ctx, job.ip)
	if err != nil && !errors.Is(err, ErrNotFound) {
		r.logError("read hostname cache", err)
		r.finish(job)
		return
	}
	gate := interestGate{}
	for {
		interests, generation := r.snapshotInterests(job)
		if len(r.enabledInterests(interests, gate.forGeneration(generation))) != 0 {
			break
		}
		if r.finishIfInterestsUnchanged(job, generation) {
			return
		}
	}
	missing := errors.Is(err, ErrNotFound)
	if current.ResolvedAt != nil && time.Since(*current.ResolvedAt) < recentResolveWindow {
		interests := r.finish(job)
		if current.Hostname != job.observed.Hostname {
			r.publish(r.enabledInterests(interests, gate.enabled), current.Hostname, current.ResolvedAt)
		}
		return
	}
	lookupCtx, cancel := context.WithTimeout(r.ctx, lookupTimeout)
	names, lookupErr := r.lookupAddr(lookupCtx, job.ip)
	cancel()
	if lookupErr != nil && !isNotFound(lookupErr) {
		r.logError("reverse DNS lookup", lookupErr)
		interests := r.finish(job)
		if missing {
			r.publish(r.enabledInterests(interests, gate.enabled), "", nil)
		}
		return
	}
	hostname := NormalizeHostname(names)
	resolvedAt := time.Now().UTC()
	if err := r.cache.Set(r.ctx, job.ip, hostname, resolvedAt); err != nil {
		r.logError("save hostname cache", err)
		r.finish(job)
		return
	}
	interests := r.finish(job)
	if current.Hostname != hostname || missing {
		r.publish(r.enabledInterests(interests, gate.enabled), hostname, &resolvedAt)
	}
}

// enabledInterests retains only users still opted into hostname resolution.
func (r *Resolver) enabledInterests(interests []interest, gate map[string]bool) []interest {
	out := make([]interest, 0, len(interests))
	for _, item := range interests {
		enabled, cached := gate[item.userID]
		if !cached {
			resolved, err := r.settingsOn(r.ctx, item.userID)
			if err != nil {
				r.logError("hostname settings gate", err, zap.String(userIDKey, item.userID))
			}
			enabled = err == nil && resolved
			gate[item.userID] = enabled
		}
		if enabled {
			out = append(out, item)
		}
	}
	return out
}

// publish emits hostname updates only to interested users.
func (r *Resolver) publish(interests []interest, hostname string, resolvedAt *time.Time) {
	if r.eventBus == nil {
		return
	}
	var timestamp any
	if resolvedAt != nil {
		timestamp = FormatTimestamp(*resolvedAt)
	}
	for _, item := range interests {
		_ = r.eventBus.Publish(r.ctx, events.AuthSessionHostnameResolved, bus.NewEvent(events.AuthSessionHostnameResolved, "hostname-resolver", map[string]any{
			userIDKey: item.userID, "ip": item.rawIP, "hostname": hostname, "resolved_at": timestamp,
		}))
	}
}

// canonicalIPs normalizes valid IP addresses while preserving their raw forms.
func canonicalIPs(ips []string) ([]string, map[string][]string) {
	byCanonical := make(map[string][]string)
	for _, raw := range ips {
		parsed := net.ParseIP(raw)
		if parsed == nil {
			continue
		}
		canonical := parsed.String()
		byCanonical[canonical] = append(byCanonical[canonical], raw)
	}
	out := make([]string, 0, len(byCanonical))
	for ip := range byCanonical {
		out = append(out, ip)
	}
	return out, byCanonical
}

// isNotFound reports whether a DNS error represents no PTR record.
func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// logError records resolver failures when a logger is configured.
func (r *Resolver) logError(message string, err error, fields ...zap.Field) {
	if err != nil && r.log != nil {
		r.log.Warn(message, append(fields, zap.Error(err))...)
	}
}
