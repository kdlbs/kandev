package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SecretStore is the subset of secrets the service needs.
type SecretStore interface {
	Reveal(ctx context.Context, id string) (string, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// Service orchestrates Slack config storage, the cached client, the
// auth-health probe, and the utility-agent run that turns a matched Slack
// message into a Kandev task. Slack is install-wide (one Slack user/team per
// Kandev install); the agent picks the destination Kandev workspace per
// message via MCP.
type Service struct {
	store   *Store
	secrets SecretStore
	runner  AgentRunner
	log     *logger.Logger

	mu        sync.Mutex
	clientFn  ClientFactory
	clients   map[string]Client
	probeHook func()
}

// AgentRunner runs the configured utility agent for a Slack match. Defined as
// an interface so tests can inject a fake without spinning up agentctl.
type AgentRunner interface {
	RunForMatch(ctx context.Context, cfg *SlackConfig, msg SlackMessage, instruction, permalink string, thread []SlackMessage) (string, error)
}

// ClientFactory builds a Client from a config + the (token, cookie) pair.
type ClientFactory func(cfg *SlackConfig, token, cookie string) Client

// DefaultClientFactory returns a real CookieClient.
func DefaultClientFactory(cfg *SlackConfig, token, cookie string) Client {
	return NewCookieClient(cfg, token, cookie)
}

// NewService wires the service. Pass nil for clientFn to use the default.
// runner may be nil — when nil, matched Slack messages are logged but no
// agent runs (useful in tests and during partial backend init).
func NewService(
	store *Store,
	secrets SecretStore,
	runner AgentRunner,
	clientFn ClientFactory,
	log *logger.Logger,
) *Service {
	if clientFn == nil {
		clientFn = DefaultClientFactory
	}
	return &Service{
		store:    store,
		secrets:  secrets,
		runner:   runner,
		log:      log,
		clientFn: clientFn,
		clients:  make(map[string]Client),
	}
}

// SetRunner wires the agent runner after construction. main.go calls this
// once the host-utility manager + utility service are available.
func (s *Service) SetRunner(r AgentRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runner = r
}

// Runner returns the wired runner (or nil).
func (s *Service) Runner() AgentRunner {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runner
}

// GetConfig returns the default workspace config enriched with HasToken/HasCookie.
func (s *Service) GetConfig(ctx context.Context) (*SlackConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns a workspace config enriched with HasToken/HasCookie.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*SlackConfig, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	cfg.HasToken = s.secretExists(ctx, SecretKeyForToken(workspaceID), SecretKeyToken, "token")
	cfg.HasCookie = s.secretExists(ctx, SecretKeyForCookie(workspaceID), SecretKeyCookie, "cookie")
	return cfg, nil
}

func (s *Service) secretExists(ctx context.Context, id, legacyID, kind string) bool {
	exists, err := s.secrets.Exists(ctx, id)
	if err != nil {
		s.log.Warn("slack: secret exists check failed",
			zap.String("kind", kind), zap.Error(err))
	}
	if exists {
		return true
	}
	exists, err = s.secrets.Exists(ctx, legacyID)
	if err != nil {
		s.log.Warn("slack: legacy secret exists check failed",
			zap.String("kind", kind), zap.Error(err))
	}
	return exists
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation.
var ErrInvalidConfig = errors.New("slack: invalid configuration")

func (s *Service) persistSecrets(ctx context.Context, workspaceID string, req *SetConfigRequest) error {
	if s.secrets == nil {
		return nil
	}
	if req.Token != "" {
		if err := s.secrets.Set(ctx, SecretKeyForToken(workspaceID), "Slack token", req.Token); err != nil {
			return fmt.Errorf("store slack token: %w", err)
		}
	}
	if req.Cookie != "" {
		if err := s.secrets.Set(ctx, SecretKeyForCookie(workspaceID), "Slack d cookie", req.Cookie); err != nil {
			return fmt.Errorf("store slack cookie: %w", err)
		}
	}
	return nil
}

// SetConfig is upsert. Empty Token/Cookie on update keeps the existing values.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*SlackConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SetConfigForWorkspace(ctx, workspaceID, req)
}

// SetConfigForWorkspace is upsert. Empty Token/Cookie on update keeps existing values.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*SlackConfig, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &SlackConfig{
		AuthMethod:          req.AuthMethod,
		CommandPrefix:       req.CommandPrefix,
		UtilityAgentID:      req.UtilityAgentID,
		PollIntervalSeconds: req.PollIntervalSeconds,
	}
	if err := s.store.UpsertConfigForWorkspace(ctx, workspaceID, cfg); err != nil {
		return nil, fmt.Errorf("upsert slack config: %w", err)
	}
	if err := s.persistSecrets(ctx, workspaceID, req); err != nil {
		return nil, err
	}
	s.invalidateClient(workspaceID)
	go func() {
		s.RecordAuthHealthForWorkspace(context.Background(), workspaceID)
	}()
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfig removes both the row and the stored secrets.
func (s *Service) DeleteConfig(ctx context.Context) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.DeleteConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace removes both the row and the stored secrets.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteConfigForWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	if s.secrets != nil {
		s.deleteSecret(ctx, SecretKeyForToken(workspaceID), "token")
		s.deleteSecret(ctx, SecretKeyForCookie(workspaceID), "cookie")
	}
	s.invalidateClient(workspaceID)
	return nil
}

func (s *Service) deleteSecret(ctx context.Context, id, kind string) {
	if err := s.secrets.Delete(ctx, id); err != nil {
		s.log.Warn("slack: secret delete failed",
			zap.String("kind", kind), zap.Error(err))
	}
}

// TestConnection validates credentials either inline (from a fresh request)
// or from the stored secrets.
func (s *Service) TestConnection(ctx context.Context, req *SetConfigRequest) (*TestConnectionResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return s.TestConnectionForWorkspace(ctx, workspaceID, req)
}

// TestConnectionForWorkspace validates credentials for one workspace.
func (s *Service) TestConnectionForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*TestConnectionResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	cfg, token, cookie, err := s.resolveCredentials(ctx, workspaceID, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client := s.clientFn(cfg, token, cookie)
	return client.AuthTest(ctx)
}

// ProbeAuth validates the stored credentials.
func (s *Service) ProbeAuth(ctx context.Context) (*TestConnectionResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return s.ProbeAuthForWorkspace(ctx, workspaceID)
}

// ProbeAuthForWorkspace validates the stored credentials for one workspace.
func (s *Service) ProbeAuthForWorkspace(ctx context.Context, workspaceID string) (*TestConnectionResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return client.AuthTest(ctx)
}

// Store exposes the underlying store so background workers can persist state.
func (s *Service) Store() *Store {
	return s.store
}

// authProbeTimeout caps a single auth-health probe.
const authProbeTimeout = 15 * time.Second

// authHealthWriteTimeout bounds the DB write that persists the probe outcome.
const authHealthWriteTimeout = 5 * time.Second

// SetProbeHook installs a callback fired at the end of each RecordAuthHealth.
func (s *Service) SetProbeHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeHook = fn
}

// RecordAuthHealth probes credentials and writes the outcome onto the row.
func (s *Service) RecordAuthHealth(ctx context.Context) {
	workspaceIDs, err := s.store.ListConfigWorkspaceIDs(ctx)
	if err != nil {
		s.log.Warn("slack: list config workspaces failed", zap.Error(err))
		return
	}
	if len(workspaceIDs) == 0 {
		s.fireProbeHook()
		return
	}
	for _, workspaceID := range workspaceIDs {
		s.RecordAuthHealthForWorkspace(ctx, workspaceID)
	}
}

// RecordAuthHealthForWorkspace probes credentials and writes the outcome onto
// one workspace row.
func (s *Service) RecordAuthHealthForWorkspace(ctx context.Context, workspaceID string) {
	workspaceID, normalizeErr := s.normalizeWorkspaceID(workspaceID)
	if normalizeErr != nil {
		s.log.Warn("slack: resolve workspace for auth health failed", zap.Error(normalizeErr))
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.ProbeAuthForWorkspace(probeCtx, workspaceID)
	ok := err == nil && res != nil && res.OK
	errMsg := ""
	switch {
	case err != nil:
		errMsg = err.Error()
	case res != nil && !res.OK:
		errMsg = res.Error
	}
	teamID, userID := "", ""
	if res != nil && ok {
		teamID = res.TeamID
		userID = res.UserID
	}
	writeCtx, writeCancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer writeCancel()
	if updateErr := s.store.UpdateAuthHealthForWorkspace(writeCtx, workspaceID, ok, errMsg, teamID, userID, time.Now().UTC()); updateErr != nil {
		s.log.Warn("slack: update auth health failed", zap.Error(updateErr))
	}
	if !ok {
		s.invalidateClient(workspaceID)
	}
	s.fireProbeHook()
}

func (s *Service) fireProbeHook() {
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// Client exposes the cached client to the trigger and runtime.
func (s *Service) Client(ctx context.Context) (Client, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.ClientForWorkspace(ctx, workspaceID)
}

// ClientForWorkspace exposes a cached client for one workspace.
func (s *Service) ClientForWorkspace(ctx context.Context, workspaceID string) (Client, error) {
	return s.clientFor(ctx, workspaceID)
}

func (s *Service) clientFor(ctx context.Context, workspaceID string) (Client, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clients[workspaceID] != nil {
		c := s.clients[workspaceID]
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	token, cookie, err := s.revealSecrets(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if token == "" || cookie == "" {
		return nil, ErrNotConfigured
	}
	client := s.clientFn(cfg, token, cookie)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clients[workspaceID] != nil {
		return s.clients[workspaceID], nil
	}
	s.clients[workspaceID] = client
	return client, nil
}

func (s *Service) revealSecrets(ctx context.Context, workspaceID string) (string, string, error) {
	if s.secrets == nil {
		return "", "", nil
	}
	token, err := s.revealSecret(ctx, SecretKeyForToken(workspaceID), SecretKeyToken)
	if err != nil {
		return "", "", fmt.Errorf("read slack token: %w", err)
	}
	cookie, err := s.revealSecret(ctx, SecretKeyForCookie(workspaceID), SecretKeyCookie)
	if err != nil {
		return "", "", fmt.Errorf("read slack cookie: %w", err)
	}
	return token, cookie, nil
}

func (s *Service) invalidateClient(workspaceID string) {
	s.mu.Lock()
	if s.clients != nil {
		delete(s.clients, workspaceID)
	}
	s.mu.Unlock()
}

func (s *Service) resolveCredentials(ctx context.Context, workspaceID string, req *SetConfigRequest) (*SlackConfig, string, string, error) {
	cfg := &SlackConfig{AuthMethod: req.AuthMethod}
	token, cookie := req.Token, req.Cookie
	if token != "" && cookie != "" {
		return cfg, token, cookie, nil
	}
	if s.secrets == nil {
		return nil, "", "", errors.New("no secret store configured")
	}
	if token == "" {
		stored, err := s.revealSecret(ctx, SecretKeyForToken(workspaceID), SecretKeyToken)
		if err != nil {
			s.log.Warn("slack: token reveal failed", zap.Error(err))
			return nil, "", "", fmt.Errorf("read slack token: %w", err)
		}
		token = stored
	}
	if cookie == "" {
		stored, err := s.revealSecret(ctx, SecretKeyForCookie(workspaceID), SecretKeyCookie)
		if err != nil {
			s.log.Warn("slack: cookie reveal failed", zap.Error(err))
			return nil, "", "", fmt.Errorf("read slack cookie: %w", err)
		}
		cookie = stored
	}
	if token == "" || cookie == "" {
		return nil, "", "", errors.New("token and cookie required — paste both to test")
	}
	return cfg, token, cookie, nil
}

func (s *Service) revealSecret(ctx context.Context, key, legacyKey string) (string, error) {
	value, err := s.secrets.Reveal(ctx, key)
	if err == nil && value != "" {
		return value, nil
	}
	legacy, legacyErr := s.secrets.Reveal(ctx, legacyKey)
	if legacyErr == nil && legacy != "" {
		return legacy, nil
	}
	if err != nil {
		return "", err
	}
	return "", legacyErr
}

func (s *Service) defaultWorkspaceID() (string, error) {
	return s.store.defaultWorkspaceID()
}

func (s *Service) normalizeWorkspaceID(workspaceID string) (string, error) {
	if workspaceID != "" {
		return workspaceID, nil
	}
	return s.defaultWorkspaceID()
}

func validateConfigRequest(req *SetConfigRequest) error {
	if req.AuthMethod == "" {
		req.AuthMethod = AuthMethodCookie
	}
	if req.AuthMethod != AuthMethodCookie {
		return fmt.Errorf("unknown auth method: %q", req.AuthMethod)
	}
	req.CommandPrefix = strings.TrimSpace(req.CommandPrefix)
	if req.CommandPrefix == "" {
		req.CommandPrefix = DefaultCommandPrefix
	}
	if req.PollIntervalSeconds == 0 {
		req.PollIntervalSeconds = DefaultPollIntervalSeconds
	}
	if req.PollIntervalSeconds < MinPollIntervalSeconds || req.PollIntervalSeconds > MaxPollIntervalSeconds {
		return fmt.Errorf("pollIntervalSeconds must be between %d and %d", MinPollIntervalSeconds, MaxPollIntervalSeconds)
	}
	return nil
}
