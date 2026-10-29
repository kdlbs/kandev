package plugins

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/plugins/manifest"
)

var errProviderDispatchClosed = errors.New("plugins: provider admission is closed")

func hasBoundedProviderCalls(m *manifest.Manifest) bool {
	return len(m.ExecutorProviders) > 0 || len(m.SandboxProviders) > 0
}

func (s *Service) beginProviderDispatch(ctx context.Context, pluginID string) (context.Context, func(), error) {
	s.providerOpMu.Lock()
	if s.providerAdmissionClosed[pluginID] {
		s.providerOpMu.Unlock()
		return nil, nil, errProviderDispatchClosed
	}
	if s.providerDispatches == nil {
		s.providerDispatches = make(map[string]map[uint64]context.CancelFunc)
	}
	s.providerDispatchID++
	dispatchID := s.providerDispatchID
	callCtx, cancel := context.WithCancel(ctx)
	if s.providerDispatches[pluginID] == nil {
		s.providerDispatches[pluginID] = make(map[uint64]context.CancelFunc)
	}
	s.providerDispatches[pluginID][dispatchID] = cancel
	s.providerOpMu.Unlock()
	return callCtx, func() {
		s.providerOpMu.Lock()
		delete(s.providerDispatches[pluginID], dispatchID)
		if len(s.providerDispatches[pluginID]) == 0 {
			delete(s.providerDispatches, pluginID)
		}
		s.providerOpMu.Unlock()
		cancel()
	}, nil
}

// closeProviderAdmission rejects calls that have not yet acquired a
// dispatch lease and cancels the contexts of calls already admitted. The
// lifecycle caller then takes the exclusive dispatch lock to drain them.
func (s *Service) closeProviderAdmission(pluginID string) {
	s.providerOpMu.Lock()
	if s.providerAdmissionClosed == nil {
		s.providerAdmissionClosed = make(map[string]bool)
	}
	s.providerAdmissionClosed[pluginID] = true
	cancels := make([]context.CancelFunc, 0, len(s.providerDispatches[pluginID]))
	for _, cancel := range s.providerDispatches[pluginID] {
		cancels = append(cancels, cancel)
	}
	s.providerOpMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) openProviderAdmission(pluginID string) {
	s.providerOpMu.Lock()
	delete(s.providerAdmissionClosed, pluginID)
	s.providerOpMu.Unlock()
}

func (s *Service) reopenProviderAdmissionIfActive(pluginID string) {
	record, err := s.Get(pluginID)
	if err == nil && record != nil && record.Status == StatusActive {
		s.openProviderAdmission(pluginID)
	}
}
