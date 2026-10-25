package plugins

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

var ErrSandboxProviderUnavailable = errors.New("plugins: sandbox provider is unavailable")

// SetSandboxProvidersEnabled applies the restart-required feature gate at startup.
func (s *Service) SetSandboxProvidersEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sandboxProvidersEnabled = enabled
}

func (s *Service) sandboxEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sandboxProvidersEnabled
}

// SandboxProviderBinding pins the installation generation selected by core.
type SandboxProviderBinding struct {
	PluginID       string    `json:"plugin_id"`
	InstallationID string    `json:"installation_id"`
	ProviderKey    string    `json:"provider_key"`
	Version        string    `json:"version"`
	InstalledAt    time.Time `json:"installed_at"`
	PackageDigest  string    `json:"package_digest"`
}

type SandboxProviderDescriptor struct {
	Binding       SandboxProviderBinding
	DisplayName   string
	PluginRunning bool
}

func (s *Service) ListSandboxProviders(ctx context.Context) ([]SandboxProviderDescriptor, error) {
	if !s.sandboxEnabled() {
		return nil, ErrSandboxProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	providers := make([]SandboxProviderDescriptor, 0)
	for _, record := range s.List() {
		remote, running := s.pluginRemote(record.ID)
		for _, declared := range record.SandboxProviders {
			providers = append(providers, SandboxProviderDescriptor{
				Binding: SandboxProviderBinding{PluginID: record.ID, InstallationID: record.InstallationID,
					ProviderKey: declared.Key, Version: record.Version, InstalledAt: record.InstalledAt, PackageDigest: record.PackageDigest},
				DisplayName:   declared.DisplayName,
				PluginRunning: running && remote != nil && record.Status == StatusActive && record.Capabilities.SandboxProvider,
			})
		}
	}
	return providers, nil
}

func (s *Service) CompileSandboxPolicy(ctx context.Context, binding SandboxProviderBinding, req *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
	return invokeSandboxProvider(ctx, s, binding, req, 10*time.Second, (*pluginsdk.RemotePlugin).CompileSandboxPolicy)
}

func (s *Service) DescribeSandboxProvider(ctx context.Context, binding SandboxProviderBinding, req *pluginsdk.DescribeSandboxProviderRequest) (*pluginsdk.DescribeSandboxProviderResponse, error) {
	return invokeSandboxProvider(ctx, s, binding, req, 5*time.Second, (*pluginsdk.RemotePlugin).DescribeSandboxProvider)
}

func (s *Service) NormalizeSandboxApproval(ctx context.Context, binding SandboxProviderBinding, req *pluginsdk.NormalizeSandboxApprovalRequest) (*pluginsdk.NormalizeSandboxApprovalResponse, error) {
	return invokeSandboxProvider(ctx, s, binding, req, 2*time.Second, (*pluginsdk.RemotePlugin).NormalizeSandboxApproval)
}

func (s *Service) NormalizeSandboxStatus(ctx context.Context, binding SandboxProviderBinding, req *pluginsdk.NormalizeSandboxStatusRequest) (*pluginsdk.NormalizeSandboxStatusResponse, error) {
	return invokeSandboxProvider(ctx, s, binding, req, 5*time.Second, (*pluginsdk.RemotePlugin).NormalizeSandboxStatus)
}

func (s *Service) ReadSandboxReport(ctx context.Context, binding SandboxProviderBinding, req *pluginsdk.ReadSandboxReportRequest) (*pluginsdk.ReadSandboxReportResponse, error) {
	return invokeSandboxProvider(ctx, s, binding, req, 5*time.Second, (*pluginsdk.RemotePlugin).ReadSandboxReport)
}

func (s *Service) ProposeSandboxRevision(ctx context.Context, binding SandboxProviderBinding, req *pluginsdk.ProposeSandboxRevisionRequest) (*pluginsdk.ProposeSandboxRevisionResponse, error) {
	return invokeSandboxProvider(ctx, s, binding, req, 10*time.Second, (*pluginsdk.RemotePlugin).ProposeSandboxRevision)
}
