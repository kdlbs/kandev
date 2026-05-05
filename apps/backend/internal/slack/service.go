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
	client    Client // singleton, cleared on config change.
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

// GetConfig returns the singleton config enriched with HasToken/HasCookie.
func (s *Service) GetConfig(ctx context.Context) (*SlackConfig, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	cfg.HasToken = s.secretExists(ctx, SecretKeyToken, "token")
	cfg.HasCookie = s.secretExists(ctx, SecretKeyCookie, "cookie")
	return cfg, nil
}

func (s *Service) secretExists(ctx context.Context, id, kind string) bool {
	exists, err := s.secrets.Exists(ctx, id)
	if err != nil {
		s.log.Warn("slack: secret exists check failed",
			zap.String("kind", kind), zap.Error(err))
	}
	return exists
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation.
var ErrInvalidConfig = errors.New("slack: invalid configuration")

func (s *Service) persistSecrets(ctx context.Context, req *SetConfigRequest) error {
	if s.secrets == nil {
		return nil
	}
	if req.Token != "" {
		if err := s.secrets.Set(ctx, SecretKeyToken, "Slack token", req.Token); err != nil {
			return fmt.Errorf("store slack token: %w", err)
		}
	}
	if req.Cookie != "" {
		if err := s.secrets.Set(ctx, SecretKeyCookie, "Slack d cookie", req.Cookie); err != nil {
			return fmt.Errorf("store slack cookie: %w", err)
		}
	}
	return nil
}

// SetConfig is upsert. Empty Token/Cookie on update keeps the existing values.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*SlackConfig, error) {
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &SlackConfig{
		AuthMethod:          req.AuthMethod,
		CommandPrefix:       req.CommandPrefix,
		UtilityAgentID:      req.UtilityAgentID,
		PollIntervalSeconds: req.PollIntervalSeconds,
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert slack config: %w", err)
	}
	if err := s.persistSecrets(ctx, req); err != nil {
		return nil, err
	}
	s.invalidateClient()
	go func() {
		s.RecordAuthHealth(context.Background())
	}()
	return s.GetConfig(ctx)
}

// DeleteConfig removes both the row and the stored secrets.
func (s *Service) DeleteConfig(ctx context.Context) error {
	if err := s.store.DeleteConfig(ctx); err != nil {
		return err
	}
	if s.secrets != nil {
		s.deleteSecret(ctx, SecretKeyToken, "token")
		s.deleteSecret(ctx, SecretKeyCookie, "cookie")
	}
	s.invalidateClient()
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
	cfg, token, cookie, err := s.resolveCredentials(ctx, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client := s.clientFn(cfg, token, cookie)
	return client.AuthTest(ctx)
}

// ProbeAuth validates the stored credentials.
func (s *Service) ProbeAuth(ctx context.Context) (*TestConnectionResult, error) {
	client, err := s.clientFor(ctx)
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
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.ProbeAuth(probeCtx)
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
	if updateErr := s.store.UpdateAuthHealth(writeCtx, ok, errMsg, teamID, userID, time.Now().UTC()); updateErr != nil {
		s.log.Warn("slack: update auth health failed", zap.Error(updateErr))
	}
	if !ok {
		s.invalidateClient()
	}
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// Client exposes the cached client to the trigger and runtime.
func (s *Service) Client(ctx context.Context) (Client, error) {
	return s.clientFor(ctx)
}

func (s *Service) clientFor(ctx context.Context) (Client, error) {
	s.mu.Lock()
	if s.client != nil {
		c := s.client
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	token, cookie, err := s.revealSecrets(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" || cookie == "" {
		return nil, ErrNotConfigured
	}
	client := s.clientFn(cfg, token, cookie)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
	s.client = client
	return client, nil
}

func (s *Service) revealSecrets(ctx context.Context) (string, string, error) {
	if s.secrets == nil {
		return "", "", nil
	}
	token, err := s.secrets.Reveal(ctx, SecretKeyToken)
	if err != nil {
		return "", "", fmt.Errorf("read slack token: %w", err)
	}
	cookie, err := s.secrets.Reveal(ctx, SecretKeyCookie)
	if err != nil {
		return "", "", fmt.Errorf("read slack cookie: %w", err)
	}
	return token, cookie, nil
}

func (s *Service) invalidateClient() {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
}

func (s *Service) resolveCredentials(ctx context.Context, req *SetConfigRequest) (*SlackConfig, string, string, error) {
	cfg := &SlackConfig{AuthMethod: req.AuthMethod}
	token, cookie := req.Token, req.Cookie
	if token != "" && cookie != "" {
		return cfg, token, cookie, nil
	}
	if s.secrets == nil {
		return nil, "", "", errors.New("no secret store configured")
	}
	if token == "" {
		stored, err := s.secrets.Reveal(ctx, SecretKeyToken)
		if err != nil {
			s.log.Warn("slack: token reveal failed", zap.Error(err))
			return nil, "", "", fmt.Errorf("read slack token: %w", err)
		}
		token = stored
	}
	if cookie == "" {
		stored, err := s.secrets.Reveal(ctx, SecretKeyCookie)
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
