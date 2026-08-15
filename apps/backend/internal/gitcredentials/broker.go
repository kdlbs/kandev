// Package gitcredentials issues opaque, revocable leases for Git HTTPS credentials.
package gitcredentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultLeaseTTL       = 12 * time.Hour
	leaseTokenBytes       = 32
	maxLeasesPerWorkspace = 10_000
)

var (
	ErrLeaseInvalid = errors.New("git credential lease is invalid")
	ErrLeaseExpired = errors.New("git credential lease is expired")
	ErrLeaseRevoked = errors.New("git credential lease was revoked")
	ErrLeaseLimit   = errors.New("git credential lease limit reached")
	ErrScopeDenied  = errors.New("git credential scope denied")
	ErrUnsupported  = errors.New("git credential provider is unsupported")
)

// Scope is the host-verified identity a lease may redeem for. Path is the
// exact HTTPS remote path supplied to a provider resolver.
type Scope struct {
	ProviderID   string
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
	Host         string
	Path         string
	TTL          time.Duration
}

// Redemption is helper-supplied, non-secret lease context. ProviderID and
// WorkspaceID are deliberately absent: the opaque lease selects those fields.
type Redemption struct {
	Lease        string
	TaskID       string
	SessionID    string
	RepositoryID string
	Host         string
	Path         string
}

// Lease is safe to pass only to the credential helper. Its token is never
// persisted or logged by this package.
type Lease struct {
	Token     string
	ExpiresAt time.Time
}

// Credential is transient material returned only after a successful lease
// redemption. Metadata is never retained in a broker lease record.
type Credential struct {
	Username  string
	Password  string
	ExpiresAt time.Time
	Metadata  any
}

// Authorizer validates task/session/repository ownership at issuance and
// redemption. It must fail closed for missing or terminal resources.
type Authorizer interface {
	AuthorizeGitCredential(context.Context, Scope) error
}

// Resolver handles one or more provider IDs. Binding returns a non-secret
// revision which changes when a connection, plugin, or credential generation
// is reset. Resolve is called for every redemption so OAuth rotation is live.
type Resolver interface {
	Supports(providerID string) bool
	Binding(context.Context, Scope) (string, error)
	Resolve(context.Context, Scope) (Credential, error)
}

// CompositeResolver selects the first live resolver that declares a provider.
// Resolvers may make their declaration dynamic, so disabled plugins stop
// resolving immediately without a restart or cached credential.
type CompositeResolver struct {
	resolvers []Resolver
}

func NewCompositeResolver(resolvers ...Resolver) *CompositeResolver {
	available := make([]Resolver, 0, len(resolvers))
	for _, resolver := range resolvers {
		if resolver != nil {
			available = append(available, resolver)
		}
	}
	return &CompositeResolver{resolvers: available}
}

func (r *CompositeResolver) Supports(providerID string) bool {
	return r.resolver(providerID) != nil
}

func (r *CompositeResolver) Binding(ctx context.Context, scope Scope) (string, error) {
	resolver := r.resolver(scope.ProviderID)
	if resolver == nil {
		return "", ErrUnsupported
	}
	return resolver.Binding(ctx, scope)
}

func (r *CompositeResolver) Resolve(ctx context.Context, scope Scope) (Credential, error) {
	resolver := r.resolver(scope.ProviderID)
	if resolver == nil {
		return Credential{}, ErrUnsupported
	}
	return resolver.Resolve(ctx, scope)
}

func (r *CompositeResolver) resolver(providerID string) Resolver {
	if r == nil {
		return nil
	}
	for _, resolver := range r.resolvers {
		if resolver.Supports(providerID) {
			return resolver
		}
	}
	return nil
}

type leaseRecord struct {
	scope     Scope
	binding   string
	ttl       time.Duration
	expiresAt time.Time
}

// Broker stores only hashed leases plus non-secret scope and revision data.
type Broker struct {
	resolver   Resolver
	authorizer Authorizer

	mu     sync.Mutex
	leases map[[sha256.Size]byte]leaseRecord
	now    func() time.Time
}

func NewBroker(resolver Resolver, authorizer Authorizer) *Broker {
	return &Broker{
		resolver: resolver, authorizer: authorizer,
		leases: make(map[[sha256.Size]byte]leaseRecord), now: time.Now,
	}
}

// Issue creates a new opaque lease after validating scope ownership and the
// live provider binding. It never resolves or stores a credential.
func (b *Broker) Issue(ctx context.Context, scope Scope) (Lease, error) {
	normalized, err := normalizeScope(scope)
	if err != nil {
		return Lease{}, err
	}
	if b == nil || b.resolver == nil || b.authorizer == nil {
		return Lease{}, ErrUnsupported
	}
	if !b.resolver.Supports(normalized.ProviderID) {
		return Lease{}, ErrUnsupported
	}
	if err := b.authorizer.AuthorizeGitCredential(ctx, normalized); err != nil {
		return Lease{}, fmt.Errorf("%w: %v", ErrScopeDenied, err)
	}
	binding, err := b.resolver.Binding(ctx, normalized)
	if err != nil {
		return Lease{}, err
	}
	if strings.TrimSpace(binding) == "" {
		return Lease{}, ErrLeaseRevoked
	}
	return b.storeLease(normalized, binding)
}

func (b *Broker) storeLease(scope Scope, binding string) (Lease, error) {
	raw := make([]byte, leaseTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return Lease{}, fmt.Errorf("create Git credential lease: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	now := b.now()
	ttl := boundedTTL(scope.TTL)
	expiresAt := now.UTC().Add(ttl)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepExpiredLocked(now)
	if b.workspaceLeaseCountLocked(scope.WorkspaceID) >= maxLeasesPerWorkspace {
		return Lease{}, fmt.Errorf("%w for workspace %s", ErrLeaseLimit, scope.WorkspaceID)
	}
	b.leases[hash] = leaseRecord{scope: scope, binding: binding, ttl: ttl, expiresAt: expiresAt}
	return Lease{Token: token, ExpiresAt: expiresAt}, nil
}

// Redeem verifies the lease and its exact helper scope, reauthorizes the live
// task resources, re-resolves provider credentials, and then renews the lease.
func (b *Broker) Redeem(ctx context.Context, request Redemption) (Credential, error) {
	if b == nil || b.resolver == nil || b.authorizer == nil {
		return Credential{}, ErrUnsupported
	}
	record, err := b.loadLease(request)
	if err != nil {
		return Credential{}, err
	}
	if err := b.authorizer.AuthorizeGitCredential(ctx, record.scope); err != nil {
		return Credential{}, fmt.Errorf("%w: %v", ErrScopeDenied, err)
	}
	if err := b.validateBinding(ctx, record); err != nil {
		return Credential{}, err
	}
	credential, err := b.resolver.Resolve(ctx, record.scope)
	if err != nil {
		return Credential{}, err
	}
	if strings.TrimSpace(credential.Username) == "" || strings.TrimSpace(credential.Password) == "" {
		return Credential{}, ErrLeaseRevoked
	}
	if err := b.validateBinding(ctx, record); err != nil {
		return Credential{}, err
	}
	if !b.renew(request.Lease) {
		return Credential{}, ErrLeaseRevoked
	}
	return credential, nil
}

func (b *Broker) loadLease(request Redemption) (leaseRecord, error) {
	if strings.TrimSpace(request.Lease) == "" {
		return leaseRecord{}, ErrLeaseInvalid
	}
	hash := sha256.Sum256([]byte(request.Lease))
	now := b.now()
	b.mu.Lock()
	record, ok := b.leases[hash]
	b.sweepExpiredLocked(now)
	b.mu.Unlock()
	if !ok {
		return leaseRecord{}, ErrLeaseInvalid
	}
	if !record.expiresAt.After(now) {
		return leaseRecord{}, ErrLeaseExpired
	}
	if !matchesRedemption(record.scope, request) {
		return leaseRecord{}, ErrScopeDenied
	}
	return record, nil
}

func (b *Broker) validateBinding(ctx context.Context, record leaseRecord) error {
	if !b.resolver.Supports(record.scope.ProviderID) {
		b.revoke(record.scope)
		return ErrLeaseRevoked
	}
	binding, err := b.resolver.Binding(ctx, record.scope)
	if err != nil {
		b.revoke(record.scope)
		return fmt.Errorf("%w: provider binding changed", ErrLeaseRevoked)
	}
	if binding == "" || binding != record.binding {
		b.revoke(record.scope)
		return ErrLeaseRevoked
	}
	return nil
}

func (b *Broker) renew(rawLease string) bool {
	hash := sha256.Sum256([]byte(rawLease))
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	record, ok := b.leases[hash]
	if !ok || !record.expiresAt.After(now) {
		return false
	}
	record.expiresAt = now.UTC().Add(record.ttl)
	b.leases[hash] = record
	return true
}

func (b *Broker) revoke(scope Scope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for hash, record := range b.leases {
		if record.scope == scope {
			delete(b.leases, hash)
		}
	}
}

func (b *Broker) RevokeTask(taskID string) {
	b.revokeMatching(func(scope Scope) bool { return scope.TaskID == taskID })
}
func (b *Broker) RevokeSession(sessionID string) {
	b.revokeMatching(func(scope Scope) bool { return scope.SessionID == sessionID })
}
func (b *Broker) RevokeWorkspace(workspaceID string) {
	b.revokeMatching(func(scope Scope) bool { return scope.WorkspaceID == workspaceID })
}
func (b *Broker) RevokeProvider(providerID string) {
	b.revokeMatching(func(scope Scope) bool { return strings.EqualFold(scope.ProviderID, providerID) })
}

// ActiveLeaseCount returns currently retained, non-expired lease records. It
// exposes no lease token, scope, or credential material.
func (b *Broker) ActiveLeaseCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepExpiredLocked(b.now())
	return len(b.leases)
}

func (b *Broker) revokeMatching(match func(Scope) bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for hash, record := range b.leases {
		if match(record.scope) {
			delete(b.leases, hash)
		}
	}
}

func (b *Broker) sweepExpiredLocked(now time.Time) {
	for hash, record := range b.leases {
		if !record.expiresAt.After(now) {
			delete(b.leases, hash)
		}
	}
}

func (b *Broker) workspaceLeaseCountLocked(workspaceID string) int {
	count := 0
	for _, record := range b.leases {
		if record.scope.WorkspaceID == workspaceID {
			count++
		}
	}
	return count
}

func normalizeScope(scope Scope) (Scope, error) {
	scope.ProviderID = strings.ToLower(strings.TrimSpace(scope.ProviderID))
	scope.WorkspaceID = strings.TrimSpace(scope.WorkspaceID)
	scope.TaskID = strings.TrimSpace(scope.TaskID)
	scope.SessionID = strings.TrimSpace(scope.SessionID)
	scope.RepositoryID = strings.TrimSpace(scope.RepositoryID)
	scope.Host = strings.ToLower(strings.TrimSpace(scope.Host))
	path, err := normalizePath(scope.Path)
	if err != nil {
		return Scope{}, err
	}
	scope.Path = path
	for name, value := range map[string]string{
		"provider": scope.ProviderID, "workspace": scope.WorkspaceID, "task": scope.TaskID,
		"session": scope.SessionID, "repository": scope.RepositoryID, "host": scope.Host, "path": scope.Path,
	} {
		if value == "" {
			return Scope{}, fmt.Errorf("%w: %s is required", ErrLeaseInvalid, name)
		}
	}
	if strings.ContainsAny(scope.Host, "/@?#") {
		return Scope{}, fmt.Errorf("%w: host is invalid", ErrLeaseInvalid)
	}
	return scope, nil
}

func normalizePath(raw string) (string, error) {
	path, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil || path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
		return "", fmt.Errorf("%w: path is invalid", ErrLeaseInvalid)
	}
	return path, nil
}

func matchesRedemption(scope Scope, request Redemption) bool {
	path, err := normalizePath(request.Path)
	if err != nil {
		return false
	}
	return scope.TaskID == strings.TrimSpace(request.TaskID) &&
		scope.SessionID == strings.TrimSpace(request.SessionID) &&
		scope.RepositoryID == strings.TrimSpace(request.RepositoryID) &&
		strings.EqualFold(scope.Host, strings.TrimSpace(request.Host)) &&
		scope.Path == path
}

func boundedTTL(requested time.Duration) time.Duration {
	if requested <= 0 || requested > defaultLeaseTTL {
		return defaultLeaseTTL
	}
	return requested
}
