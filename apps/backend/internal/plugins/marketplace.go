package plugins

import "github.com/kandev/kandev/internal/plugins/marketplace"

// SetMarketplace attaches the plugin-discovery catalog service. Called by
// Provide after the source store is built; nil-safe callers must check
// Marketplace() before use (a Service constructed via NewService in tests has
// no marketplace unless one is set).
func (s *Service) SetMarketplace(m *marketplace.Service) {
	s.mu.Lock()
	s.marketplace = m
	s.mu.Unlock()
}

// Marketplace returns the attached catalog service, or nil when the
// marketplace subsystem is unavailable (e.g. its source store failed to
// initialize). Handlers guard on nil and return 503.
func (s *Service) Marketplace() *marketplace.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.marketplace
}

// InstalledForMarketplace maps the current registry into the minimal
// id+version facts the catalog needs to compute each entry's install state.
func (s *Service) InstalledForMarketplace() []marketplace.InstalledPlugin {
	records := s.List()
	out := make([]marketplace.InstalledPlugin, 0, len(records))
	for _, rec := range records {
		out = append(out, marketplace.InstalledPlugin{ID: rec.ID, Version: rec.Version})
	}
	return out
}
