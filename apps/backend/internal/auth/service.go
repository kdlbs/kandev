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

const (
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

// BackfillFunc claims pre-auth unowned resources (workspaces, secrets) for the
// admin created by the setup wizard. Wired in backendapp to avoid coupling the
// auth service to the task/secrets packages.
type BackfillFunc func(ctx context.Context, ownerID string) error

// Deps are the service dependencies.
type Deps struct {
	Cfg       *config.Config
	Store     *store.Store
	Users     userstore.AccountRepository
	Backfills []BackfillFunc
	Log       *logger.Logger
}

// Service implements authentication flows and mode resolution.
//
// Authentication is turned on/off through the `features.auth` runtime feature
// flag (Settings > System > Feature Toggles, or KANDEV_FEATURES_AUTH). The
// effective flag value has already been resolved into cfg.Features.Auth at
// startup (env > DB override > profile default), so the service simply reads
// it — there is no separate auth-enable setting.
type Service struct {
	cfg       *config.Config
	store     *store.Store
	users     userstore.AccountRepository
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

// refreshMode recomputes the mode cache from the effective `features.auth`
// flag and the presence of an admin identity:
//
//	flag off              → Disabled (single-user, today's behavior)
//	flag on, no admin yet → Setup    (first visitor runs the wizard)
//	flag on, admin exists → Enabled
func (s *Service) refreshMode(ctx context.Context) error {
	if !s.cfg.Features.Auth {
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
