package org

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Errors surfaced to callers.
var (
	// ErrTenancyRequiresAuth reports the one configuration that cannot work: a
	// tenant boundary with no identity behind it.
	ErrTenancyRequiresAuth = errors.New(
		"features.multiTenancy requires features.auth: organizations need authenticated users to belong to them")
	ErrOrgSuspended       = errors.New("organization is suspended")
	ErrSlugMismatch       = errors.New("the typed slug does not match the organization")
	ErrCannotDeleteLast   = errors.New("the last organization cannot be deleted")
	ErrCannotDeleteActive = errors.New("stop the organization's running work before deleting it")
	ErrNameRequired       = errors.New("organization name is required")
)

// DataMigrator assigns pre-tenancy rows to the default organization. The task
// repository implements it; the org package stays free of a task dependency.
type DataMigrator interface {
	// AssignWorkspacesWithoutOrg returns how many workspaces it moved.
	AssignWorkspacesWithoutOrg(ctx context.Context, orgID string) (int64, error)
	// DropCrossOrgWorkspaceMembers removes membership rows whose member and
	// workspace would land in different organizations, and returns how many.
	DropCrossOrgWorkspaceMembers(ctx context.Context) (int64, error)
	// DeleteOrgData removes every row belonging to an organization.
	DeleteOrgData(ctx context.Context, orgID string) error
}

// AccountMigrator is the user-store half of the same migration.
type AccountMigrator interface {
	AssignUsersWithoutOrg(ctx context.Context, orgID string) (int64, error)
	SetOperator(ctx context.Context, id string, operator bool) error
	CountOperators(ctx context.Context) (int, error)
	FirstAdminID(ctx context.Context) (string, error)
	DeleteUsersByOrg(ctx context.Context, orgID string) error
}

// FirstAdminCreator provisions an organization's first administrator. An
// ordinary admin can only create accounts in their own tenant, so a brand-new
// organization would otherwise have no way to get its first user; this is the
// operator-only path that breaks that circularity.
type FirstAdminCreator func(ctx context.Context, orgID, email, password, displayName string) error

// Service owns organization lifecycle and the tenancy migration.
type Service struct {
	store      *Store
	accounts   AccountMigrator
	data       DataMigrator
	log        *logger.Logger
	enabled    bool
	firstAdmin FirstAdminCreator
}

// SetFirstAdminCreator installs the operator-only first-admin path.
func (s *Service) SetFirstAdminCreator(create FirstAdminCreator) { s.firstAdmin = create }

// CreateFirstAdmin provisions the first administrator of an organization.
func (s *Service) CreateFirstAdmin(ctx context.Context, orgID, email, password, displayName string) error {
	if s.firstAdmin == nil {
		return errors.New("account provisioning is unavailable")
	}
	if _, err := s.store.Get(ctx, orgID); err != nil {
		return err
	}
	if err := s.firstAdmin(ctx, orgID, email, password, displayName); err != nil {
		return err
	}
	s.log.Info("organization first admin created", zap.String("org_id", orgID))
	return nil
}

// NewService builds the org service. enabled mirrors features.multiTenancy.
func NewService(store *Store, accounts AccountMigrator, data DataMigrator, enabled bool, log *logger.Logger) *Service {
	return &Service{store: store, accounts: accounts, data: data, enabled: enabled,
		log: log.WithFields(zap.String("component", "org-service"))}
}

// Enabled reports whether organizations are on.
func (s *Service) Enabled() bool { return s != nil && s.enabled }

// ValidateStartup refuses the one combination that cannot work.
func ValidateStartup(multiTenancy, auth bool) error {
	if multiTenancy && !auth {
		return ErrTenancyRequiresAuth
	}
	return nil
}

// EnsureDefaultOrg runs the tenancy migration: it creates the single default
// organization if none exists, puts every unassigned user and workspace into
// it, drops membership rows that would straddle two organizations, and grants
// the operator tier to the first admin so an upgraded instance is never left
// with nobody able to manage organizations.
//
// It is idempotent, and it is a no-op when organizations are off.
func (s *Service) EnsureDefaultOrg(ctx context.Context, instanceName string) (*Org, error) {
	if !s.Enabled() {
		return nil, nil
	}
	existing, err := s.store.Default(ctx)
	if err != nil && !errors.Is(err, ErrOrgNotFound) {
		return nil, err
	}
	if existing == nil {
		name := strings.TrimSpace(instanceName)
		if name == "" {
			name = "Default organization"
		}
		existing, err = s.store.Create(ctx, name, Slugify(name), true)
		if err != nil {
			return nil, fmt.Errorf("create default organization: %w", err)
		}
		s.log.Info("created the default organization",
			zap.String("org_id", existing.ID), zap.String("slug", existing.Slug))
	}

	users, err := s.accounts.AssignUsersWithoutOrg(ctx, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("assign users to the default organization: %w", err)
	}
	workspaces, err := s.data.AssignWorkspacesWithoutOrg(ctx, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("assign workspaces to the default organization: %w", err)
	}
	dropped, err := s.data.DropCrossOrgWorkspaceMembers(ctx)
	if err != nil {
		return nil, fmt.Errorf("drop cross-organization memberships: %w", err)
	}
	if users > 0 || workspaces > 0 || dropped > 0 {
		s.log.Info("tenancy migration applied",
			zap.Int64("users_assigned", users),
			zap.Int64("workspaces_assigned", workspaces),
			zap.Int64("cross_org_memberships_dropped", dropped))
	}

	if err := s.ensureOperator(ctx); err != nil {
		return nil, err
	}
	return existing, nil
}

// ensureOperator grants the operator tier to the first admin when nobody holds
// it. Without this an upgraded instance would have organizations and no way to
// manage them.
func (s *Service) ensureOperator(ctx context.Context) error {
	count, err := s.accounts.CountOperators(ctx)
	if err != nil || count > 0 {
		return err
	}
	adminID, err := s.accounts.FirstAdminID(ctx)
	if err != nil || adminID == "" {
		return err
	}
	if err := s.accounts.SetOperator(ctx, adminID, true); err != nil {
		return err
	}
	s.log.Info("granted the instance operator tier to the first admin", zap.String("user_id", adminID))
	return nil
}

// Get returns an organization by ID.
func (s *Service) Get(ctx context.Context, id string) (*Org, error) { return s.store.Get(ctx, id) }

// List returns every organization.
func (s *Service) List(ctx context.Context) ([]*Org, error) { return s.store.List(ctx) }

// Create adds an organization.
func (s *Service) Create(ctx context.Context, name, slug string) (*Org, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	if slug = Slugify(slug); slug == "org" {
		slug = Slugify(name)
	}
	org, err := s.store.Create(ctx, name, slug, false)
	if err != nil {
		return nil, err
	}
	s.log.Info("organization created", zap.String("org_id", org.ID), zap.String("slug", org.Slug))
	return org, nil
}

// SetStatus suspends or resumes an organization. Suspension is reversible and
// retains every row and file; it is the lever for a billing lapse.
func (s *Service) SetStatus(ctx context.Context, id, status string) (*Org, error) {
	org, err := s.store.UpdateNameStatus(ctx, id, "", status)
	if err != nil {
		return nil, err
	}
	s.log.Info("organization status changed",
		zap.String("org_id", id), zap.String("status", org.Status))
	return org, nil
}

// Rename changes an organization's display name. The slug is immutable
// because it is the confirmation token for deletion.
func (s *Service) Rename(ctx context.Context, id, name string) (*Org, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrNameRequired
	}
	return s.store.UpdateNameStatus(ctx, id, strings.TrimSpace(name), "")
}

// Delete removes an organization and all of its data. It requires the caller
// to type the slug, refuses the last remaining organization, and removes the
// org row last so a failure part-way leaves the org still identifiable.
func (s *Service) Delete(ctx context.Context, id, confirmSlug string) error {
	org, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if confirmSlug != org.Slug {
		return ErrSlugMismatch
	}
	count, err := s.store.Count(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrCannotDeleteLast
	}
	if err := s.data.DeleteOrgData(ctx, id); err != nil {
		return fmt.Errorf("delete organization data: %w", err)
	}
	if err := s.accounts.DeleteUsersByOrg(ctx, id); err != nil {
		return fmt.Errorf("delete organization accounts: %w", err)
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("organization deleted", zap.String("org_id", id), zap.String("slug", org.Slug))
	return nil
}

// ClaimSetupAdmin places the account created by the authentication setup
// wizard into the default organization and grants it the instance operator
// tier. It is a no-op when organizations are off.
func (s *Service) ClaimSetupAdmin(ctx context.Context, userID string) error {
	if !s.Enabled() || userID == "" {
		return nil
	}
	defaultOrg, err := s.EnsureDefaultOrg(ctx, "Default organization")
	if err != nil || defaultOrg == nil {
		return err
	}
	if err := s.accounts.SetOperator(ctx, userID, true); err != nil {
		return err
	}
	s.log.Info("setup admin placed in the default organization",
		zap.String("user_id", userID), zap.String("org_id", defaultOrg.ID))
	return nil
}
