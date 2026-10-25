package sandbox

import "context"

// ProviderView describes an installed adapter, not observed confinement.
type ProviderView struct {
	InstallationID string `json:"installation_id"`
	ProviderKey    string `json:"provider_key"`
	DisplayName    string `json:"display_name"`
	Version        string `json:"version"`
	Available      bool   `json:"available"`
}

// SetProviderCatalog binds installed provider metadata at startup.
func (s *Service) SetProviderCatalog(catalog func(context.Context) ([]ProviderView, error)) {
	s.providerCatalog = catalog
}

func (s *Service) ListProviders(ctx context.Context) ([]ProviderView, error) {
	if _, err := s.operator(ctx); err != nil {
		return nil, err
	}
	if s.providerCatalog == nil {
		return []ProviderView{}, nil
	}
	providers, err := s.providerCatalog(ctx)
	if err != nil {
		return nil, err
	}
	if providers == nil {
		providers = []ProviderView{}
	}
	return providers, nil
}
