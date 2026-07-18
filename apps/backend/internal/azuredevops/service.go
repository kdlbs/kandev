package azuredevops

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

var (
	// ErrInvalidConfig identifies user-supplied connection settings that cannot
	// be persisted or tested.
	ErrInvalidConfig = errors.New("azure devops: invalid configuration")
	// ErrInvalidWorkspaceID prevents operations from falling back across
	// workspace boundaries when the caller omits its scope.
	ErrInvalidWorkspaceID = errors.New("azure devops: workspace id is required")
	organizationNameRE    = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,48}[A-Za-z0-9])?$`)
)

const (
	authProbeTimeout       = 15 * time.Second
	authHealthWriteTimeout = 5 * time.Second
)

// SecretStore is the encrypted secret-store surface used by this integration.
type SecretStore interface {
	Reveal(ctx context.Context, id string) (string, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// Service coordinates workspace configuration, encrypted PATs, and auth
// probes.
type Service struct {
	store      *Store
	secrets    SecretStore
	clientFn   ClientFactory
	log        *logger.Logger
	mock       *MockClient
	repoLookup RepositoryLookup
}

// MockClient returns the E2E mock when the provider is in mock mode.
func (s *Service) MockClient() *MockClient { return s.mock }

// NewService constructs the Azure DevOps configuration service.
func NewService(store *Store, secrets SecretStore, clientFn ClientFactory, log *logger.Logger) *Service {
	if clientFn == nil {
		clientFn = DefaultClientFactory
	}
	if log == nil {
		log = logger.Default()
	}
	return &Service{store: store, secrets: secrets, clientFn: clientFn, log: log}
}

// Store exposes the configuration store to health-poller adapters.
func (s *Service) Store() *Store {
	return s.store
}

// ValidateOrganizationURL accepts only canonical Azure DevOps Services
// organization URLs: https://dev.azure.com/<organization>.
func ValidateOrganizationURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "dev.azure.com" {
		return "", errors.New("organizationUrl must use https://dev.azure.com")
	}
	if parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", errors.New("organizationUrl must be a canonical organization URL")
	}
	organization := strings.TrimPrefix(parsed.Path, "/")
	if parsed.Path != "/"+organization || !organizationNameRE.MatchString(organization) || parsed.String() != raw {
		return "", errors.New("organizationUrl must contain exactly one valid organization name")
	}
	return raw, nil
}

// GetConfigForWorkspace returns a redacted workspace configuration.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*Config, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil || cfg == nil || s.secrets == nil {
		return cfg, err
	}
	cfg.HasSecret, err = s.secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("check azure devops PAT: %w", err)
	}
	return cfg, nil
}

// SetConfigForWorkspace upserts a workspace connection. An empty PAT retains
// the existing encrypted credential.
func (s *Service) SetConfigForWorkspace(
	ctx context.Context,
	workspaceID string,
	req *SetConfigRequest,
) (*Config, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	cfg, err := configFromRequest(workspaceID, req)
	if err != nil {
		return nil, err
	}
	if req.PAT == "" {
		if err := s.requireStoredPAT(ctx, workspaceID); err != nil {
			return nil, err
		}
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert azure devops config: %w", err)
	}
	if req.PAT != "" {
		if s.secrets == nil {
			return nil, errors.New("azure devops: no secret store configured")
		}
		if err := s.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Azure DevOps PAT", req.PAT); err != nil {
			return nil, fmt.Errorf("store azure devops PAT: %w", err)
		}
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace removes both configuration and its encrypted PAT.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	if err := s.store.DeleteConfig(ctx, workspaceID); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			s.log.Warn("azure devops: secret delete failed", zap.Error(err))
		}
	}
	return nil
}

// TestConnectionForWorkspace probes submitted credentials without persisting
// them, or falls back to that workspace's stored config and PAT.
func (s *Service) TestConnectionForWorkspace(
	ctx context.Context,
	workspaceID string,
	req *SetConfigRequest,
) (*TestConnectionResult, error) {
	cfg, pat, err := s.resolveCredentials(ctx, workspaceID, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return s.clientFn(cfg, pat).TestAuth(ctx)
}

// ProbeAuthForWorkspace tests the workspace's stored credentials.
func (s *Service) ProbeAuthForWorkspace(ctx context.Context, workspaceID string) (*TestConnectionResult, error) {
	return s.TestConnectionForWorkspace(ctx, workspaceID, &SetConfigRequest{})
}

// RecordAuthHealth probes every configured workspace and persists each result.
func (s *Service) RecordAuthHealth(ctx context.Context) {
	workspaceIDs, err := s.store.ListConfigWorkspaceIDs(ctx)
	if err != nil {
		s.log.Warn("azure devops: list configured workspaces failed", zap.Error(err))
		return
	}
	for _, workspaceID := range workspaceIDs {
		s.RecordAuthHealthForWorkspace(ctx, workspaceID)
	}
}

// RecordAuthHealthForWorkspace probes and persists one workspace's status.
func (s *Service) RecordAuthHealthForWorkspace(ctx context.Context, workspaceID string) {
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	result, err := s.ProbeAuthForWorkspace(probeCtx, workspaceID)
	cancel()
	ok, errMsg := authHealthResult(result, err)
	writeCtx, writeCancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer writeCancel()
	if err := s.store.UpdateAuthHealth(writeCtx, workspaceID, ok, errMsg, time.Now().UTC()); err != nil {
		s.log.Warn("azure devops: update auth health failed", zap.Error(err))
	}
}

func (s *Service) resolveCredentials(
	ctx context.Context,
	workspaceID string,
	req *SetConfigRequest,
) (*Config, string, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, "", err
	}
	if req == nil {
		req = &SetConfigRequest{}
	}
	var cfg *Config
	var err error
	if req.OrganizationURL != "" {
		cfg, err = configFromRequest(workspaceID, req)
	} else {
		cfg, err = s.store.GetConfig(ctx, workspaceID)
		if err == nil && cfg == nil {
			err = ErrNotConfigured
		}
	}
	if err != nil {
		return nil, "", err
	}
	pat := req.PAT
	if pat == "" {
		pat, err = s.revealPAT(ctx, workspaceID)
		if err != nil {
			return nil, "", err
		}
	}
	return cfg, pat, nil
}

func (s *Service) requireStoredPAT(ctx context.Context, workspaceID string) error {
	if s.secrets == nil {
		return errors.New("azure devops: no secret store configured")
	}
	exists, err := s.secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil {
		return fmt.Errorf("check azure devops PAT: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: pat required", ErrInvalidConfig)
	}
	return nil
}

func (s *Service) revealPAT(ctx context.Context, workspaceID string) (string, error) {
	if s.secrets == nil {
		return "", ErrNotConfigured
	}
	pat, err := s.secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil || pat == "" {
		if err != nil {
			return "", fmt.Errorf("read azure devops PAT: %w", err)
		}
		return "", ErrNotConfigured
	}
	return pat, nil
}

func configFromRequest(workspaceID string, req *SetConfigRequest) (*Config, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request required", ErrInvalidConfig)
	}
	organizationURL, err := ValidateOrganizationURL(req.OrganizationURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	authMethod := req.AuthMethod
	if authMethod == "" {
		authMethod = AuthMethodPAT
	}
	if authMethod != AuthMethodPAT {
		return nil, fmt.Errorf("%w: authMethod must be pat", ErrInvalidConfig)
	}
	return &Config{
		WorkspaceID:        workspaceID,
		OrganizationURL:    organizationURL,
		DefaultProjectID:   req.DefaultProjectID,
		DefaultProjectName: req.DefaultProjectName,
		AuthMethod:         authMethod,
	}, nil
}

func validateWorkspaceID(workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrInvalidWorkspaceID
	}
	return nil
}

func authHealthResult(result *TestConnectionResult, err error) (bool, string) {
	if err != nil {
		return false, err.Error()
	}
	if result == nil {
		return false, "azure devops: empty authentication response"
	}
	if !result.OK {
		return false, result.Error
	}
	return true, ""
}
