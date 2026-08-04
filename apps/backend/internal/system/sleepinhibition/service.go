package sleepinhibition

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"go.uber.org/zap"
)

const defaultReconcileInterval = 30 * time.Second

type Option func(*Service)

func WithReconcileInterval(interval time.Duration) Option {
	return func(service *Service) {
		if interval > 0 {
			service.reconcileInterval = interval
		}
	}
}

type Service struct {
	store     *Store
	sessions  SessionReader
	inhibitor Inhibitor
	eventBus  bus.EventBus
	logger    *logger.Logger

	reconcileInterval time.Duration
	reconcileMu       sync.Mutex
	mu                sync.Mutex
	status            Status
	lease             Lease
	cancel            context.CancelFunc
	done              chan struct{}
	signal            chan struct{}
	subscription      bus.Subscription
	wg                sync.WaitGroup
}

func NewService(store *Store, sessions SessionReader, inhibitor Inhibitor, eventBus bus.EventBus, log *logger.Logger, options ...Option) *Service {
	service := &Service{
		store:             store,
		sessions:          sessions,
		inhibitor:         inhibitor,
		eventBus:          eventBus,
		logger:            log,
		reconcileInterval: defaultReconcileInterval,
		status:            statusFor(inhibitor),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Start(ctx context.Context) error {
	if s.store == nil || s.sessions == nil || s.inhibitor == nil || s.eventBus == nil {
		return errors.New("sleep inhibition service dependencies are unavailable")
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.signal = make(chan struct{}, 1)
	s.mu.Unlock()

	subscription, err := s.eventBus.Subscribe(events.TaskSessionStateChanged, s.handleSessionEvent)
	if err != nil {
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.done = nil
		s.signal = nil
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.subscription = subscription
	done := s.done
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run(runCtx, done)
	s.signalReconcile()
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	subscription := s.subscription
	s.mu.Unlock()
	if subscription != nil {
		_ = subscription.Unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	s.wg.Wait()

	s.releaseLease(context.Background())
	s.mu.Lock()
	s.cancel = nil
	s.done = nil
	s.signal = nil
	s.subscription = nil
	s.mu.Unlock()
}

func (s *Service) Get(ctx context.Context) (Response, error) {
	settings, err := s.loadSettings(ctx)
	if err != nil {
		if IsInvalidPersisted(err) {
			s.warn("Ignoring invalid persisted task sleep inhibition settings", err)
			settings = Settings{}
		} else {
			return Response{}, err
		}
	}
	s.mu.Lock()
	status := s.status
	s.mu.Unlock()
	return Response{Settings: settings, Status: status}, nil
}

func (s *Service) Update(ctx context.Context, settings Settings) (Response, error) {
	if s.store == nil {
		return Response{}, errors.New("sleep inhibition settings store is unavailable")
	}
	if err := s.store.Save(ctx, settings); err != nil {
		return Response{}, err
	}
	s.reconcile(ctx)
	return s.Get(ctx)
}

func (s *Service) handleSessionEvent(context.Context, *bus.Event) error {
	s.signalReconcile()
	return nil
}

func (s *Service) signalReconcile() {
	s.mu.Lock()
	signal := s.signal
	s.mu.Unlock()
	if signal == nil {
		return
	}
	select {
	case signal <- struct{}{}:
	default:
	}
}

func (s *Service) run(ctx context.Context, done chan struct{}) {
	defer s.wg.Done()
	defer close(done)
	ticker := time.NewTicker(s.reconcileInterval)
	defer ticker.Stop()
	var leaseDone <-chan error
	for {
		leaseDone = s.reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-s.signalChannel():
		case <-ticker.C:
		case err, ok := <-leaseDone:
			s.handleLeaseExit(err, ok)
		}
	}
}

func (s *Service) signalChannel() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.signal
}

func (s *Service) reconcile(ctx context.Context) <-chan error {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	settings, err := s.loadSettings(ctx)
	if err != nil {
		if IsInvalidPersisted(err) {
			s.warn("Ignoring invalid persisted task sleep inhibition settings", err)
			settings = Settings{}
		} else {
			s.warn("Unable to load task sleep inhibition settings", err)
			return s.currentLeaseDone()
		}
	}

	if !settings.Enabled {
		s.clearLeaseStatus(ctx)
		return nil
	}

	if !s.probeCapability(ctx) {
		return s.currentLeaseDone()
	}

	working, err := s.hasWorkingSession(ctx)
	if err != nil {
		s.warn("Unable to read active task sessions for sleep inhibition", err)
		return s.currentLeaseDone()
	}

	if !working {
		s.clearLeaseStatus(ctx)
		return nil
	}

	s.mu.Lock()
	lease := s.lease
	s.mu.Unlock()
	if lease != nil {
		s.setStatus(func(status *Status) {
			status.Active = true
			status.Issue = ""
		})
		return lease.Done()
	}

	if !s.inhibitor.Supported() {
		s.setStatus(func(status *Status) {
			status.Active = false
			status.Issue = IssueUnsupportedPlatform
		})
		return nil
	}

	lease, err = s.inhibitor.Acquire(ctx)
	if err != nil {
		issue := IssueFromError(err)
		s.warn("Unable to acquire task sleep inhibition", err, zap.String("issue", string(issue)))
		s.setStatus(func(status *Status) {
			status.Active = false
			status.Issue = issue
		})
		return nil
	}
	s.mu.Lock()
	s.lease = lease
	s.mu.Unlock()
	s.setStatus(func(status *Status) {
		status.Active = true
		status.Issue = ""
	})
	return lease.Done()
}

type capabilityProber interface {
	Probe(context.Context) error
}

func (s *Service) probeCapability(ctx context.Context) bool {
	prober, ok := s.inhibitor.(capabilityProber)
	if !ok {
		return true
	}
	if err := prober.Probe(ctx); err != nil {
		s.mu.Lock()
		lease := s.lease
		s.mu.Unlock()
		if lease == nil {
			s.setStatus(func(status *Status) {
				status.Active = false
				status.Issue = IssueFromError(err)
			})
		}
		s.warn("Unable to probe task sleep inhibition service", err)
		return false
	}
	return true
}

func (s *Service) clearLeaseStatus(ctx context.Context) {
	s.releaseLease(ctx)
	s.setStatus(func(status *Status) {
		status.Active = false
		if status.Supported {
			status.Issue = ""
		} else {
			status.Issue = IssueUnsupportedPlatform
		}
	})
}

func (s *Service) hasWorkingSession(ctx context.Context) (bool, error) {
	sessions, err := s.sessions.ListActiveTaskSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, session := range sessions {
		if session != nil && sessionstate.IsWorking(session.State) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) currentLeaseDone() <-chan error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lease == nil {
		return nil
	}
	return s.lease.Done()
}

func (s *Service) handleLeaseExit(err error, ok bool) {
	s.mu.Lock()
	if s.lease == nil {
		s.mu.Unlock()
		return
	}
	s.lease = nil
	s.status.Active = false
	switch {
	case err != nil:
		s.status.Issue = IssueFromError(err)
	case ok:
		s.status.Issue = IssueRequestFailed
	default:
		s.status.Issue = IssueRequestFailed
	}
	s.mu.Unlock()
	if err != nil {
		s.warn("Task sleep inhibition lease ended unexpectedly", err)
	}
	s.signalReconcile()
}

func (s *Service) releaseLease(ctx context.Context) {
	s.mu.Lock()
	lease := s.lease
	s.lease = nil
	s.mu.Unlock()
	if lease == nil {
		return
	}
	if err := lease.Release(); err != nil {
		s.warn("Unable to release task sleep inhibition", err)
	}
	_ = ctx
}

func (s *Service) setStatus(update func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.status)
}

func (s *Service) loadSettings(ctx context.Context) (Settings, error) {
	if s.store == nil {
		return Settings{}, errors.New("sleep inhibition settings store is unavailable")
	}
	return s.store.Load(ctx)
}

func (s *Service) warn(message string, err error, fields ...zap.Field) {
	if s.logger == nil {
		return
	}
	if err != nil {
		fields = append([]zap.Field{zap.Error(err)}, fields...)
	}
	s.logger.Warn(message, fields...)
}

func statusFor(inhibitor Inhibitor) Status {
	if inhibitor == nil {
		return Status{Platform: PlatformOther, Supported: false}
	}
	status := Status{Platform: inhibitor.Platform(), Supported: inhibitor.Supported()}
	if !status.Supported {
		status.Issue = IssueUnsupportedPlatform
	}
	return status
}
