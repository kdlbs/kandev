package provideraccess

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/github"
)

// VerifiedLease is resolved from current Host records for this plugin
// connection. Its fields cannot be copied from a plugin request.
type VerifiedLease struct {
	LeaseID             string
	GrantID             string
	Expected            FinalLeaseIdentity
	InstallationID      int64
	CanonicalRepository string
}

// LeaseAuthority verifies the connected plugin, current managed session,
// approval, linked target, and provider connection on every call.
type LeaseAuthority interface {
	VerifyLease(context.Context, string) (VerifiedLease, error)
}

type RerunTokenSource interface {
	Mint(context.Context, int64, string) (github.InstallationToken, error)
	Revoke(context.Context, string) error
}

type activeToken struct {
	grantID     string
	workspaceID string
	value       string
	expiresAt   time.Time
}

// Runtime owns only transient exact-token revocation material. Its ledger
// remains authoritative after process loss; a restart never recreates tokens.
type Runtime struct {
	store     *Store
	authority LeaseAuthority
	tokens    RerunTokenSource
	mu        sync.Mutex
	active    map[string]activeToken
}

func NewRuntime(store *Store, authority LeaseAuthority, tokens RerunTokenSource) (*Runtime, error) {
	if store == nil || authority == nil || tokens == nil {
		return nil, errors.New("provider access runtime dependencies are required")
	}
	return &Runtime{store: store, authority: authority, tokens: tokens,
		active: make(map[string]activeToken)}, nil
}

// Redeem returns one token only after a durable one-shot claim, fresh
// authority recheck, and serialized final exposure admission.
func (r *Runtime) Redeem(ctx context.Context, leaseID string) (github.InstallationToken, error) {
	first, err := r.authority.VerifyLease(ctx, leaseID)
	if err != nil {
		return github.InstallationToken{}, ErrGrantUnavailable
	}
	if !validVerifiedLease(first, leaseID) {
		return github.InstallationToken{}, ErrGrantUnavailable
	}
	if _, err := r.store.ClaimMintIntent(ctx, MintClaim{
		LeaseID: leaseID, GrantID: first.GrantID, Expected: first.Expected,
	}); err != nil {
		return github.InstallationToken{}, err
	}
	token, err := r.tokens.Mint(ctx, first.InstallationID, first.CanonicalRepository)
	if err != nil {
		return github.InstallationToken{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, verifyErr := r.authority.VerifyLease(ctx, leaseID)
	if verifyErr != nil || current != first {
		return github.InstallationToken{}, r.revokeUnexported(ctx, token.Token, ErrGrantUnavailable)
	}
	receipt := ExposureReceipt{
		LeaseID: leaseID, GrantID: first.GrantID, Provider: "github",
		ProviderPrincipalID: token.Principal.PrincipalID,
		RepositoryID:        first.CanonicalRepository,
		PermissionProfile:   "github_actions_rerun",
		ProviderExpiresAt:   token.ExpiresAt, Expected: first.Expected,
	}
	if err := r.store.RecordExposureOrRevoke(ctx, receipt, func(revokeCtx context.Context) error {
		return r.tokens.Revoke(revokeCtx, token.Token)
	}); err != nil {
		return github.InstallationToken{}, err
	}
	r.active[leaseID] = activeToken{grantID: first.GrantID,
		workspaceID: first.Expected.Scope.WorkspaceID,
		value:       token.Token, expiresAt: token.ExpiresAt}
	return token, nil
}

func validVerifiedLease(lease VerifiedLease, requestedID string) bool {
	return lease.LeaseID == requestedID && lease.GrantID != "" &&
		lease.InstallationID > 0 && lease.CanonicalRepository != "" &&
		lease.Expected.Scope.RepositoryID == lease.CanonicalRepository &&
		lease.Expected.Scope.Provider == "github" &&
		lease.Expected.Scope.Purpose == "actions_write"
}

func (r *Runtime) revokeUnexported(ctx context.Context, token string, cause error) error {
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if r.tokens.Revoke(revokeCtx, token) != nil {
		return errors.Join(cause, ErrRevocationUnconfirmed)
	}
	return cause
}

// RevokeGrant fences the ledger and revokes every still-held token for that
// grant. Provider failure remains visible in the exposure receipt.
func (r *Runtime) RevokeGrant(ctx context.Context, workspaceID, grantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.store.RevokeGrant(ctx, workspaceID, grantID, time.Now().UTC()); err != nil {
		return err
	}
	var result error
	for leaseID, token := range r.active {
		if token.grantID == grantID && token.workspaceID == workspaceID {
			result = errors.Join(result, r.revokeActive(ctx, leaseID, token))
		}
	}
	return result
}

// Release revokes the exact exported token and records the provider outcome.
func (r *Runtime) Release(ctx context.Context, leaseID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.active[leaseID]
	if !ok {
		return ErrRevocationUnconfirmed
	}
	return r.revokeActive(ctx, leaseID, token)
}

func (r *Runtime) revokeActive(ctx context.Context, leaseID string, token activeToken) error {
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	confirmed := r.tokens.Revoke(revokeCtx, token.value) == nil
	if err := r.store.RecordRevocationResult(revokeCtx, leaseID, time.Now().UTC(), confirmed); err != nil {
		return err
	}
	if !confirmed {
		return ErrRevocationUnconfirmed
	}
	delete(r.active, leaseID)
	return nil
}

// FenceWorkspace preserves unexpired residual receipts and revokes every
// exact token still held by this Host. Missing in-memory tokens are reported
// by the returned IDs; they cannot be reconstructed after a restart.
func (r *Runtime) FenceWorkspace(ctx context.Context, workspaceID string) (WorkspaceFenceResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result, err := r.store.FenceWorkspace(ctx, workspaceID)
	if err != nil {
		return WorkspaceFenceResult{}, err
	}
	for _, leaseID := range result.ExposedLeaseIDs {
		if token, ok := r.active[leaseID]; ok {
			err = errors.Join(err, r.revokeActive(ctx, leaseID, token))
		}
	}
	return result, err
}
