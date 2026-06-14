package metrics

import (
	"context"
	"sync"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type Broadcaster func(*ws.Message)

type AgentctlMetricsClient interface {
	SystemMetrics(ctx context.Context, metricIDs []string, diskPath string) (*SourceSnapshot, error)
}

type ExecutionSource struct {
	ID           string
	Label        string
	ExecutorType string
	SessionID    string
	TaskID       string
	Client       AgentctlMetricsClient
}

type ExecutionProvider interface {
	MetricExecutions() []ExecutionSource
}

const executionMetricsDiskPath = "/"

type Service struct {
	store     *Store
	collector *Collector

	mu          sync.Mutex
	interested  map[string]struct{}
	cancel      context.CancelFunc
	broadcaster Broadcaster
	executions  ExecutionProvider
}

func NewService(store *Store, collector *Collector) *Service {
	return &Service{
		store:      store,
		collector:  collector,
		interested: make(map[string]struct{}),
	}
}

func (s *Service) SetBroadcaster(b Broadcaster) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcaster = b
}

func (s *Service) SetExecutionProvider(provider ExecutionProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions = provider
}

func (s *Service) GetSettings(ctx context.Context) (GlobalSettings, error) {
	return s.store.GetSettings(ctx)
}

func (s *Service) SaveSettings(ctx context.Context, settings GlobalSettings) (GlobalSettings, error) {
	return s.store.SaveSettings(ctx, settings)
}

func (s *Service) MetricsSubscribe(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.interested[clientID]; ok {
		return
	}
	s.interested[clientID] = struct{}{}
	if len(s.interested) == 1 {
		s.startLocked()
	}
}

func (s *Service) MetricsUnsubscribe(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.interested, clientID)
	if len(s.interested) == 0 && s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *Service) startLocked() {
	if s.cancel != nil {
		return
	}
	s.collector.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.run(ctx)
}

func (s *Service) run(ctx context.Context) {
	for {
		settings, err := s.store.GetSettings(ctx)
		if err == nil {
			s.publish(ctx, settings)
		}
		interval := time.Duration(DefaultIntervalSeconds) * time.Second
		if err == nil {
			interval = time.Duration(settings.IntervalSeconds) * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) publish(ctx context.Context, settings GlobalSettings) {
	s.mu.Lock()
	broadcaster := s.broadcaster
	hasInterest := len(s.interested) > 0
	s.mu.Unlock()
	if broadcaster == nil || !hasInterest {
		return
	}
	snapshot := s.collector.SampleWithTimestamp(ctx, settings)
	if settings.CollectExecution {
		snapshot.Sources = append(snapshot.Sources, s.collectExecutions(ctx, settings)...)
	}
	msg, err := ws.NewNotification(ws.ActionSystemMetricsUpdated, snapshot)
	if err != nil {
		return
	}
	broadcaster(msg)
}

func (s *Service) collectExecutions(ctx context.Context, settings GlobalSettings) []SourceSnapshot {
	s.mu.Lock()
	provider := s.executions
	s.mu.Unlock()
	if provider == nil {
		return nil
	}
	executions := provider.MetricExecutions()
	sources := make([]SourceSnapshot, 0, len(executions))
	for _, execution := range executions {
		if execution.Client == nil {
			continue
		}
		source, err := execution.Client.SystemMetrics(ctx, settings.Metrics, executionMetricsDiskPath)
		if err != nil {
			sources = append(sources, unavailableExecutionSource(execution.ID, execution.Label, "execution", err.Error()))
			continue
		}
		source.ID = execution.ID
		source.Label = execution.Label
		source.Kind = "execution"
		source.ExecutorType = execution.ExecutorType
		source.SessionID = execution.SessionID
		source.TaskID = execution.TaskID
		sources = append(sources, *source)
	}
	return sources
}
