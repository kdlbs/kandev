// Package auth implements kandev's opt-in user authentication: local
// email+password identities (OIDC-ready schema), DB-backed browser sessions,
// personal access tokens, invites, and the auth-mode state machine.
//
// Opt-in contract: when the feature is disabled the HTTP/WS middleware injects
// a synthetic admin identity for the pre-auth single user, so downstream code
// is identity-aware while behavior stays byte-identical to the unauthenticated
// product.
package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// Mode is the effective authentication mode of the instance.
type Mode string

const (
	// ModeDisabled preserves today's unauthenticated single-user behavior.
	ModeDisabled Mode = "disabled"
	// ModeSetup means auth is required but no admin identity exists yet: the
	// first visitor completes the setup wizard and becomes the admin.
	ModeSetup Mode = "setup"
	// ModeEnabled means auth is fully enforced.
	ModeEnabled Mode = "enabled"
)

// settingsKeyMode is the system-settings key persisting the opt-in toggle.
const settingsKeyMode = "auth.mode"

const (
	modeValueEnabled = "enabled"

	minPasswordLength = 8

	loginRateWindow   = 5 * time.Minute
	loginRateAttempts = 10

	// sessionTouchInterval throttles sliding-expiry writes.
	sessionTouchInterval = time.Minute

	maxUserAgentLen = 256
)

// Sentinel errors surfaced to HTTP handlers.
var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrRateLimited        = errors.New("too many attempts; try again later")
	ErrSetupNotAvailable  = errors.New("setup is not available")
	ErrInviteInvalid      = errors.New("invite is invalid, expired, or already used")
	ErrLastAdmin          = errors.New("cannot remove or disable the last active admin")
	ErrEmailTaken         = errors.New("email is already in use")
	ErrValidation         = errors.New("invalid input")
)

// SettingsStore is the narrow slice of internal/system/settings used here.
type SettingsStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, value []byte) error
}

// BackfillFunc claims pre-auth unowned resources (workspaces, secrets) for the
// admin created by the setup wizard. Wired in backendapp to avoid coupling the
// auth service to the task/secrets packages.
type BackfillFunc func(ctx context.Context, ownerID string) error

// Deps are the service dependencies.
type Deps struct {
	Cfg       *config.Config
	Store     *store.Store
	Users     userstore.AccountRepository
	Settings  SettingsStore
	Backfills []BackfillFunc
	Log       *logger.Logger
}

// Service implements authentication flows and mode resolution.
type Service struct {
	cfg       *config.Config
	store     *store.Store
	users     userstore.AccountRepository
	settings  SettingsStore
	backfills []BackfillFunc
	log       *logger.Logger
	limiter   *loginLimiter
	mode      atomic.Value // Mode

	// setupMu serializes the setup wizard so two concurrent first-visitors
	// cannot both mutate the shared default-user profile before the identity
	// insert commits.
	setupMu sync.Mutex

	// dummyHash equalizes login timing for unknown emails.
	dummyHash string
}

// NewService constructs the service and computes the initial mode.
func NewService(ctx context.Context, deps Deps) (*Service, error) {
	dummy, err := HashPassword("kandev-timing-equalizer")
	if err != nil {
		return nil, err
	}
	s := &Service{
		cfg:       deps.Cfg,
		store:     deps.Store,
		users:     deps.Users,
		settings:  deps.Settings,
		backfills: deps.Backfills,
		log:       deps.Log,
		limiter:   newLoginLimiter(loginRateWindow, loginRateAttempts),
		dummyHash: dummy,
	}
	if err := s.refreshMode(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Mode returns the cached effective auth mode.
func (s *Service) Mode() Mode {
	if v, ok := s.mode.Load().(Mode); ok {
		return v
	}
	return ModeDisabled
}

// CookieName returns the configured session cookie name.
func (s *Service) CookieName() string {
	if name := strings.TrimSpace(s.cfg.Auth.CookieName); name != "" {
		return name
	}
	return "kandev_session"
}

// SessionTTL returns the sliding session lifetime.
func (s *Service) SessionTTL() time.Duration {
	hours := s.cfg.Auth.SessionTTLHours
	if hours <= 0 {
		hours = 720
	}
	return time.Duration(hours) * time.Hour
}

// refreshMode recomputes the mode cache from env + persisted setting + the
// presence of an admin identity.
func (s *Service) refreshMode(ctx context.Context) error {
	required := s.cfg.Auth.Required
	if !required {
		raw, ok, err := s.settings.Get(ctx, settingsKeyMode)
		if err != nil {
			return err
		}
		required = ok && strings.TrimSpace(string(raw)) == modeValueEnabled
	}
	if !required {
		s.mode.Store(ModeDisabled)
		return nil
	}
	count, err := s.store.CountAdminIdentities(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		s.mode.Store(ModeSetup)
	} else {
		s.mode.Store(ModeEnabled)
	}
	return nil
}

// SetEnabled flips the persisted opt-in toggle and returns the new mode.
// Enabling on an instance with no admin identity yields ModeSetup — the
// caller should immediately run the setup wizard.
func (s *Service) SetEnabled(ctx context.Context, enabled bool) (Mode, error) {
	value := "disabled"
	if enabled {
		value = modeValueEnabled
	}
	if err := s.settings.Save(ctx, settingsKeyMode, []byte(value)); err != nil {
		return s.Mode(), err
	}
	if err := s.refreshMode(ctx); err != nil {
		return s.Mode(), err
	}
	return s.Mode(), nil
}

// EnvRequired reports whether KANDEV_AUTH_REQUIRED forces authentication on
// (the settings toggle cannot disable it then).
func (s *Service) EnvRequired() bool { return s.cfg.Auth.Required }

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmailPassword(email, password string) error {
	if email == "" || !strings.Contains(email, "@") {
		return ErrValidation
	}
	if len(password) < minPasswordLength {
		return ErrValidation
	}
	return nil
}

func roleOf(user *usermodels.User) authn.Role {
	if user.Role == usermodels.RoleAdmin {
		return authn.RoleAdmin
	}
	return authn.RoleMember
}

func truncateUserAgent(ua string) string {
	if len(ua) > maxUserAgentLen {
		return ua[:maxUserAgentLen]
	}
	return ua
}
