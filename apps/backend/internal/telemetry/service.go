package telemetry

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

const (
	defaultFlushInterval     = 30 * time.Second
	defaultHeartbeatInterval = 24 * time.Hour
	defaultQueueSize         = 256
	defaultMaxBatch          = 50
	sendTimeout              = 10 * time.Second
)

// Options configures a Service. Zero values fall back to production
// defaults; tests inject short intervals and a fake Sink.
type Options struct {
	Endpoint          string
	APIKey            string
	Debug             bool
	EnvDisabled       bool
	Version           string
	DeployMode        string
	FlushInterval     time.Duration
	HeartbeatInterval time.Duration
	QueueSize         int
	MaxBatch          int
	Sink              Sink
}

// Service owns the consent record, the event queue, the bus collector,
// and the background flush/heartbeat goroutines. Every path is
// fail-silent: telemetry must never delay or break product flows.
type Service struct {
	opts  Options
	store SettingsStore
	bus   bus.EventBus
	log   *logger.Logger

	queue chan Event

	mu        sync.Mutex
	consent   ConsentStatus
	installID string
	started   bool
	cancel    context.CancelFunc
	subs      []bus.Subscription

	wg sync.WaitGroup
}

// Provide constructs the Service from app config. Failures are non-fatal
// for the backend; the caller logs and continues without telemetry.
func Provide(cfg *config.Config, log *logger.Logger, pool *db.Pool, eventBus bus.EventBus, version string) (*Service, error) {
	store, err := systemsettings.NewStore(pool)
	if err != nil {
		return nil, err
	}
	return NewService(store, eventBus, log, Options{
		Endpoint:    cfg.Telemetry.Endpoint,
		APIKey:      cfg.Telemetry.APIKey,
		Debug:       cfg.Telemetry.Debug,
		EnvDisabled: EnvDisabled(),
		Version:     version,
		DeployMode:  detectDeployMode(),
	}), nil
}

// NewService builds a Service with defaults applied. Consent is loaded
// from the store in Start.
func NewService(store SettingsStore, eventBus bus.EventBus, log *logger.Logger, opts Options) *Service {
	if opts.Endpoint == "" {
		opts.Endpoint = defaultEndpoint
	}
	if opts.APIKey == "" {
		opts.APIKey = defaultAPIKey
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = defaultFlushInterval
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = defaultHeartbeatInterval
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultQueueSize
	}
	if opts.MaxBatch <= 0 {
		opts.MaxBatch = defaultMaxBatch
	}
	if opts.Sink == nil {
		opts.Sink = newPostHogSink(opts.Endpoint, opts.APIKey)
	}
	return &Service{
		opts:    opts,
		store:   store,
		bus:     eventBus,
		log:     log.WithFields(zap.String("component", "telemetry")),
		queue:   make(chan Event, opts.QueueSize),
		consent: ConsentUnasked,
	}
}

// Start loads consent, subscribes the collector, and launches the flush
// and heartbeat loops. A hard env kill switch means Start does nothing at
// all — no subscriptions, no goroutines — beyond loading consent so the
// HTTP surface can still report the stored preference.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	if err := s.loadConsent(ctx); err != nil {
		s.log.Warn("telemetry: failed to load consent; treating as unasked", zap.Error(err))
	}
	if s.opts.EnvDisabled {
		s.log.Debug("telemetry: disabled by environment")
		return
	}

	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	s.subscribeCollector()

	s.wg.Add(2)
	go s.flushLoop(loopCtx)
	go s.heartbeatLoop(loopCtx)
}

// Stop cancels the loops, detaches the collector, and makes a best-effort
// final flush. Safe to call more than once.
func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	subs := s.subs
	s.subs = nil
	s.started = false
	s.mu.Unlock()

	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	if cancel == nil {
		return
	}
	cancel()
	s.wg.Wait()

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer flushCancel()
	s.flushOnce(flushCtx)
}

// subscribeCollector attaches one bus subscription per allowlisted
// domain event. The handlers forward nothing from the payload — the
// event name is the entire signal.
func (s *Service) subscribeCollector() {
	if s.bus == nil {
		return
	}
	var subs []bus.Subscription
	for subject, name := range busEventAllowlist {
		sub, err := s.bus.Subscribe(subject, func(context.Context, *bus.Event) error {
			s.enqueue(Event{Name: name})
			return nil
		})
		if err != nil {
			s.log.Debug("telemetry: subscribe failed", zap.String("subject", subject), zap.Error(err))
			continue
		}
		subs = append(subs, sub)
	}
	s.mu.Lock()
	s.subs = subs
	s.mu.Unlock()
}

// EnqueueUI validates and queues a batch of frontend-submitted events.
// Returns how many were accepted; invalid entries are silently dropped.
func (s *Service) EnqueueUI(submissions []UIEventSubmission) int {
	accepted := 0
	for i, submission := range submissions {
		if i >= maxUIEventsPerRequest {
			break
		}
		event, ok := sanitizeUIEvent(submission.Name, submission.Properties)
		if !ok {
			continue
		}
		s.enqueue(event)
		accepted++
	}
	return accepted
}

// UIEventSubmission is one entry of POST /api/v1/telemetry/events.
type UIEventSubmission struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

// emissionAllowed is the single gate every enqueue passes through:
// hard env switches first, then explicit granted consent.
func (s *Service) emissionAllowed() bool {
	if s.opts.EnvDisabled {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.consent == ConsentGranted
}

func (s *Service) enqueue(event Event) {
	if !s.emissionAllowed() {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	select {
	case s.queue <- event:
	default:
		s.log.Debug("telemetry: queue full, dropping event", zap.String("event", event.Name))
	}
}

func (s *Service) drainQueue() {
	for {
		select {
		case <-s.queue:
		default:
			return
		}
	}
}

func (s *Service) flushLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.opts.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flushOnce(ctx)
		}
	}
}

// flushOnce drains up to MaxBatch queued events and hands them to the
// sink. Failures drop the batch: no retries, no persistence, no impact
// on the product. Intentionally package-internal so deterministic tests
// can drive it without timers.
func (s *Service) flushOnce(ctx context.Context) {
	batch := s.collectBatch()
	if len(batch) == 0 {
		return
	}
	state := s.Consent()
	if state.Status != ConsentGranted || state.InstallID == "" {
		return
	}
	s.debugLogBatch(batch)
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
	defer cancel()
	if err := s.opts.Sink.Send(sendCtx, state.InstallID, batch); err != nil {
		s.log.Debug("telemetry: send failed, dropping batch",
			zap.Int("events", len(batch)), zap.Error(err))
	}
}

func (s *Service) collectBatch() []Event {
	var batch []Event
	base := s.baseProperties()
	for len(batch) < s.opts.MaxBatch {
		select {
		case event := <-s.queue:
			merged := make(map[string]string, len(base)+len(event.Properties))
			for k, v := range base {
				merged[k] = v
			}
			for k, v := range event.Properties {
				merged[k] = v
			}
			event.Properties = merged
			batch = append(batch, event)
		default:
			return batch
		}
	}
	return batch
}

// debugLogBatch prints the exact outgoing payload when
// KANDEV_TELEMETRY_DEBUG is on, so users can inspect what leaves the
// machine (Turborepo-style transparency).
func (s *Service) debugLogBatch(batch []Event) {
	if !s.opts.Debug {
		return
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return
	}
	s.log.Info("telemetry: sending batch", zap.ByteString("payload", payload))
}

func (s *Service) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()
	s.enqueue(Event{Name: EventInstallHeartbeat})
	ticker := time.NewTicker(s.opts.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.enqueue(Event{Name: EventInstallHeartbeat})
		}
	}
}
