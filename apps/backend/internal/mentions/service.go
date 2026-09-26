package mentions

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type registeredProvider struct {
	descriptor ProviderDescriptor
	provider   MentionProvider
	authorizer ReferenceAuthorizer
	owner      string
}

// Registry stores provider descriptors independently of their implementation type.
type Registry struct {
	mu                   sync.RWMutex
	providers            []registeredProvider
	sources              map[string]struct{}
	providerKinds        map[string]struct{}
	referenceAuthorizers map[string]ReferenceAuthorizer
	refreshers           []SourceRefresher
}

func NewRegistry() *Registry {
	return &Registry{
		sources:              make(map[string]struct{}),
		providerKinds:        make(map[string]struct{}),
		referenceAuthorizers: make(map[string]ReferenceAuthorizer),
	}
}

func (r *Registry) Register(provider MentionProvider) error {
	registered, err := newRegisteredProvider("", provider)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureAvailable(registered); err != nil {
		return err
	}
	r.addRegistered(registered)
	return nil
}

// ReplaceOwner atomically replaces every dynamic provider owned by owner.
// The registry snapshot sees either the complete previous set or the complete
// replacement, never a partially refreshed plugin manifest.
func (r *Registry) ReplaceOwner(owner string, providers ...MentionProvider) error {
	owner = strings.TrimSpace(owner)
	if !validIdentity(owner) {
		return fmt.Errorf("%w: owner is required", ErrInvalidDescriptor)
	}
	replacement, err := newRegisteredProviders(owner, providers)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	retained := providersWithoutOwner(r.providers, owner)
	sources, providerKinds, authorizers := providerIndexes(retained)
	if err := ensureRegisteredProvidersAvailable(replacement, sources, providerKinds); err != nil {
		return err
	}
	for _, registered := range replacement {
		providerKey := referenceProviderKey(registered.descriptor.Provider, registered.descriptor.Kind)
		sources[registered.descriptor.Source] = struct{}{}
		providerKinds[providerKey] = struct{}{}
		authorizers[providerKey] = registered.authorizer
	}
	retained = append(retained, replacement...)
	r.providers = retained
	r.sources = sources
	r.providerKinds = providerKinds
	r.referenceAuthorizers = authorizers
	return nil
}

// UnregisterOwner removes every dynamic provider owned by owner.
func (r *Registry) UnregisterOwner(owner string) error {
	return r.ReplaceOwner(owner)
}

// RegisterSourceRefresher adds a dynamic source refresher. Registry reads run
// refreshers before taking their lock-protected snapshots or authorizer lookups.
func (r *Registry) RegisterSourceRefresher(refresher SourceRefresher) {
	if refresher == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshers = append(r.refreshers, refresher)
}

func (r *Registry) refreshDynamicSources() {
	r.mu.RLock()
	refreshers := append([]SourceRefresher(nil), r.refreshers...)
	r.mu.RUnlock()
	for _, refresher := range refreshers {
		refresher.RefreshMentionSources()
	}
}

func newRegisteredProviders(owner string, providers []MentionProvider) ([]registeredProvider, error) {
	registered := make([]registeredProvider, 0, len(providers))
	for _, provider := range providers {
		entry, err := newRegisteredProvider(owner, provider)
		if err != nil {
			return nil, err
		}
		registered = append(registered, entry)
	}
	return registered, nil
}

func newRegisteredProvider(owner string, provider MentionProvider) (registeredProvider, error) {
	if provider == nil {
		return registeredProvider{}, fmt.Errorf("%w: provider is nil", ErrInvalidDescriptor)
	}
	descriptor, err := normalizeDescriptor(provider.Descriptor())
	if err != nil {
		return registeredProvider{}, err
	}
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok || authorizer == nil {
		return registeredProvider{}, ErrMissingAuthorizer
	}
	return registeredProvider{descriptor: descriptor, provider: provider, authorizer: authorizer, owner: owner}, nil
}

func (r *Registry) ensureAvailable(registered registeredProvider) error {
	if r.sources == nil {
		r.sources = make(map[string]struct{})
	}
	if r.providerKinds == nil {
		r.providerKinds = make(map[string]struct{})
	}
	if _, exists := r.sources[registered.descriptor.Source]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateSource, registered.descriptor.Source)
	}
	providerKey := referenceProviderKey(registered.descriptor.Provider, registered.descriptor.Kind)
	if _, exists := r.providerKinds[providerKey]; exists {
		return fmt.Errorf("%w: %s/%s", ErrDuplicateProvider, registered.descriptor.Provider, registered.descriptor.Kind)
	}
	return nil
}

func (r *Registry) addRegistered(registered registeredProvider) {
	if r.referenceAuthorizers == nil {
		r.referenceAuthorizers = make(map[string]ReferenceAuthorizer)
	}
	providerKey := referenceProviderKey(registered.descriptor.Provider, registered.descriptor.Kind)
	r.providers = append(r.providers, registered)
	r.sources[registered.descriptor.Source] = struct{}{}
	r.providerKinds[providerKey] = struct{}{}
	r.referenceAuthorizers[providerKey] = registered.authorizer
}

func providersWithoutOwner(providers []registeredProvider, owner string) []registeredProvider {
	retained := make([]registeredProvider, 0, len(providers))
	for _, provider := range providers {
		if provider.owner != owner {
			retained = append(retained, provider)
		}
	}
	return retained
}

func providerIndexes(providers []registeredProvider) (
	map[string]struct{}, map[string]struct{}, map[string]ReferenceAuthorizer,
) {
	sources := make(map[string]struct{}, len(providers))
	providerKinds := make(map[string]struct{}, len(providers))
	authorizers := make(map[string]ReferenceAuthorizer, len(providers))
	for _, provider := range providers {
		providerKey := referenceProviderKey(provider.descriptor.Provider, provider.descriptor.Kind)
		sources[provider.descriptor.Source] = struct{}{}
		providerKinds[providerKey] = struct{}{}
		authorizers[providerKey] = provider.authorizer
	}
	return sources, providerKinds, authorizers
}

func ensureRegisteredProvidersAvailable(
	providers []registeredProvider,
	sources map[string]struct{},
	providerKinds map[string]struct{},
) error {
	for _, provider := range providers {
		if _, exists := sources[provider.descriptor.Source]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateSource, provider.descriptor.Source)
		}
		providerKey := referenceProviderKey(provider.descriptor.Provider, provider.descriptor.Kind)
		if _, exists := providerKinds[providerKey]; exists {
			return fmt.Errorf("%w: %s/%s", ErrDuplicateProvider, provider.descriptor.Provider, provider.descriptor.Kind)
		}
		// Record accepted replacements as we go. This retains exact duplicate
		// detection while making large plugin manifests linear rather than
		// rescanning every preceding descriptor.
		sources[provider.descriptor.Source] = struct{}{}
		providerKinds[providerKey] = struct{}{}
	}
	return nil
}

func referenceProviderKey(provider, kind string) string {
	return provider + "\x00" + kind
}

// AuthorizeReference dispatches to the provider registered for the normalized identity.
func (r *Registry) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	r.refreshDynamicSources()
	key := referenceProviderKey(request.Reference.Provider, request.Reference.Kind)
	r.mu.RLock()
	authorizer, ok := r.referenceAuthorizers[key]
	r.mu.RUnlock()
	if !ok {
		return ErrReferenceProviderUnavailable
	}
	return authorizer.AuthorizeReference(ctx, request)
}

// AuthorizeForWorkspace satisfies entityrefs.WorkspaceAuthorizer for message submission.
func (r *Registry) AuthorizeForWorkspace(
	ctx context.Context,
	workspaceID string,
	reference apiv1.EntityReference,
) error {
	return r.AuthorizeReference(ctx, ReferenceAuthorizationRequest{
		WorkspaceID: workspaceID,
		Purpose:     ReferencePurposeSubmission,
		Reference:   reference,
	})
}

var descriptorIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,127}$`)

func normalizeDescriptor(descriptor ProviderDescriptor) (ProviderDescriptor, error) {
	descriptor.Source = strings.TrimSpace(descriptor.Source)
	descriptor.Provider = strings.TrimSpace(descriptor.Provider)
	descriptor.Kind = strings.TrimSpace(descriptor.Kind)
	if !descriptorIDPattern.MatchString(descriptor.Source) ||
		!descriptorIDPattern.MatchString(descriptor.Provider) ||
		!descriptorIDPattern.MatchString(descriptor.Kind) {
		return ProviderDescriptor{}, fmt.Errorf("%w: source, provider, and kind must be namespaced lowercase identifiers", ErrInvalidDescriptor)
	}
	descriptor.DisplayName = strings.TrimSpace(descriptor.DisplayName)
	descriptor.KindLabel = strings.TrimSpace(descriptor.KindLabel)
	if !validDescriptorLabel(descriptor.DisplayName) || !validDescriptorLabel(descriptor.KindLabel) {
		return ProviderDescriptor{}, fmt.Errorf("%w: labels must be valid UTF-8 and at most 100 characters", ErrInvalidDescriptor)
	}
	if descriptor.DisplayName == "" {
		descriptor.DisplayName = descriptor.Provider
	}
	if descriptor.KindLabel == "" {
		descriptor.KindLabel = mentionLabelWorkItem
	}
	return descriptor, nil
}

func validDescriptorLabel(label string) bool {
	return utf8.ValidString(label) && utf8.RuneCountInString(label) <= 100
}

func (r *Registry) snapshot() []registeredProvider {
	r.refreshDynamicSources()
	r.mu.RLock()
	defer r.mu.RUnlock()
	providers := append([]registeredProvider(nil), r.providers...)
	sort.SliceStable(providers, func(i, j int) bool {
		if providers[i].descriptor.Order == providers[j].descriptor.Order {
			return providers[i].descriptor.Source < providers[j].descriptor.Source
		}
		return providers[i].descriptor.Order < providers[j].descriptor.Order
	})
	return providers
}

// Service validates and aggregates every registered provider.
type Service struct {
	registry         *Registry
	log              *logger.Logger
	providerTimeout  time.Duration
	maxConcurrency   int
	providerSlots    chan struct{}
	providerSlotMu   sync.Mutex
	providerInFlight map[string]chan struct{}
}

const (
	defaultProviderTimeout = 1500 * time.Millisecond
	defaultMaxConcurrency  = 4
)

// Option configures aggregate search execution.
type Option func(*Service)

func WithProviderTimeout(timeout time.Duration) Option {
	return func(service *Service) {
		if timeout > 0 {
			service.providerTimeout = timeout
		}
	}
}

func WithMaxConcurrency(limit int) Option {
	return func(service *Service) {
		if limit > 0 {
			service.maxConcurrency = limit
		}
	}
}

func WithLogger(log *logger.Logger) Option {
	return func(service *Service) {
		service.log = log
	}
}

func NewService(registry *Registry, options ...Option) *Service {
	service := &Service{
		registry:        registry,
		providerTimeout: defaultProviderTimeout,
		maxConcurrency:  defaultMaxConcurrency,
	}
	for _, option := range options {
		option(service)
	}
	if service.maxConcurrency <= 0 {
		service.maxConcurrency = defaultMaxConcurrency
	}
	service.providerSlots = make(chan struct{}, service.maxConcurrency)
	service.providerInFlight = make(map[string]chan struct{})
	return service
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (*apiv1.MentionSearchResponse, error) {
	request, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	response := &apiv1.MentionSearchResponse{
		Query:  request.Query,
		Groups: []apiv1.MentionGroup{},
	}
	providers := s.providers()
	if len(providers) == 0 {
		return response, nil
	}
	response.Groups = s.searchProviders(ctx, providers, request)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) providers() []registeredProvider {
	if s.registry == nil {
		return nil
	}
	return s.registry.snapshot()
}

func (s *Service) searchProviders(
	ctx context.Context,
	providers []registeredProvider,
	request SearchRequest,
) []apiv1.MentionGroup {
	groups := make([]apiv1.MentionGroup, len(providers))
	var wg sync.WaitGroup
	for index := range providers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			groups[index] = s.searchProvider(ctx, providers[index], request)
		}()
	}
	wg.Wait()
	return groups
}

func failedGroup(descriptor ProviderDescriptor, status Status) apiv1.MentionGroup {
	group := groupFromDescriptor(descriptor)
	group.Status = status
	return group
}

func (s *Service) searchProvider(ctx context.Context, registered registeredProvider, request SearchRequest) apiv1.MentionGroup {
	waitCtx, cancelWait := context.WithTimeout(ctx, s.providerTimeout)
	providerSlot := s.slotForProvider(registered.descriptor.Source)
	select {
	case providerSlot <- struct{}{}:
		cancelWait()
	case <-waitCtx.Done():
		cancelWait()
		return failedGroup(registered.descriptor, StatusTimeout)
	}
	select {
	case s.providerSlots <- struct{}{}:
	case <-ctx.Done():
		<-providerSlot
		return failedGroup(registered.descriptor, StatusTimeout)
	}
	providerCtx, cancelProvider := context.WithTimeout(ctx, s.providerTimeout)
	defer cancelProvider()
	result := make(chan apiv1.MentionGroup, 1)
	go func() {
		group := searchProviderWithinContext(providerCtx, registered, request, s.log)
		// The provider slot belongs to the underlying call. A provider that ignores
		// cancellation stays quarantined without consuming shared search capacity.
		<-providerSlot
		result <- group
	}()
	select {
	case group := <-result:
		<-s.providerSlots
		return group
	case <-providerCtx.Done():
		<-s.providerSlots
		return failedGroup(registered.descriptor, StatusTimeout)
	}
}

func (s *Service) slotForProvider(source string) chan struct{} {
	s.providerSlotMu.Lock()
	defer s.providerSlotMu.Unlock()
	if s.providerInFlight == nil {
		s.providerInFlight = make(map[string]chan struct{})
	}
	if slot, ok := s.providerInFlight[source]; ok {
		return slot
	}
	slot := make(chan struct{}, 1)
	s.providerInFlight[source] = slot
	return slot
}

func searchProviderWithinContext(
	ctx context.Context,
	registered registeredProvider,
	request SearchRequest,
	log *logger.Logger,
) apiv1.MentionGroup {
	group := groupFromDescriptor(registered.descriptor)
	candidates, err := registered.provider.Search(ctx, request)
	if err != nil {
		status := classifyProviderError(err)
		if log != nil && !errors.Is(err, context.Canceled) {
			log.Warn(
				"mention provider search failed",
				zap.String("source", registered.descriptor.Source),
				zap.String("provider", registered.descriptor.Provider),
				zap.String("kind", registered.descriptor.Kind),
				zap.String("status", string(status)),
			)
		}
		group.Status = status
		return group
	}
	if ctx.Err() != nil {
		return failedGroup(registered.descriptor, StatusTimeout)
	}
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if len(group.Results) == request.Limit {
			break
		}
		candidate, ok := normalizeCandidate(candidate)
		if !ok {
			continue
		}
		reference := referenceFromCandidate(registered.descriptor, candidate)
		if registered.authorizer != nil {
			if err := registered.authorizer.AuthorizeReference(ctx, ReferenceAuthorizationRequest{
				WorkspaceID: request.WorkspaceID,
				Purpose:     ReferencePurposeSearch,
				Reference:   reference,
			}); err != nil {
				continue
			}
		}
		if _, duplicate := seen[reference.Ref]; duplicate {
			continue
		}
		seen[reference.Ref] = struct{}{}
		group.Results = append(group.Results, reference)
	}
	return group
}

func classifyProviderError(err error) Status {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return StatusTimeout
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && safeFailureStatus(providerErr.Status) {
		return providerErr.Status
	}
	return StatusUpstreamError
}

func safeFailureStatus(status Status) bool {
	switch status {
	case StatusNotConfigured, StatusUnauthorized, StatusRateLimited, StatusTimeout,
		StatusUpstreamError, StatusUnsupportedScope:
		return true
	default:
		return false
	}
}

func normalizeCandidate(candidate Candidate) (Candidate, bool) {
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.Scope = strings.TrimSpace(candidate.Scope)
	if !validIdentity(candidate.ID) || !validIdentity(candidate.Scope) {
		return Candidate{}, false
	}
	candidate.Title = normalizeDisplayText(candidate.Title)
	candidate.Key = normalizeDisplayText(candidate.Key)
	if candidate.Title == "" || utf8.RuneCountInString(candidate.Title) > 500 ||
		utf8.RuneCountInString(candidate.Key) > 200 {
		return Candidate{}, false
	}
	candidate.URL = strings.TrimSpace(candidate.URL)
	if !safeMentionURL(candidate.URL) {
		return Candidate{}, false
	}
	return candidate, true
}

func validIdentity(value string) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 512 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func normalizeDisplayText(value string) string {
	if !utf8.ValidString(value) {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func safeMentionURL(raw string) bool {
	if raw == "" || !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 2048 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "" && parsed.Host == "" {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func normalizeRequest(request SearchRequest) (SearchRequest, error) {
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	request.Query = strings.TrimSpace(request.Query)
	if !validIdentity(request.WorkspaceID) {
		return SearchRequest{}, fmt.Errorf("%w: workspace ID is required", ErrInvalidRequest)
	}
	if !utf8.ValidString(request.Query) {
		return SearchRequest{}, fmt.Errorf("%w: query must be valid UTF-8", ErrInvalidRequest)
	}
	queryLength := utf8.RuneCountInString(request.Query)
	if queryLength < 1 || queryLength > 200 {
		return SearchRequest{}, fmt.Errorf("%w: query must contain 1 to 200 characters", ErrInvalidRequest)
	}
	for _, character := range request.Query {
		if unicode.IsControl(character) {
			return SearchRequest{}, fmt.Errorf("%w: query must not contain control characters", ErrInvalidRequest)
		}
	}
	switch {
	case request.Limit == 0:
		request.Limit = DefaultLimit
	case request.Limit < 1:
		request.Limit = 1
	case request.Limit > MaxLimit:
		request.Limit = MaxLimit
	}
	return request, nil
}

func groupFromDescriptor(descriptor ProviderDescriptor) apiv1.MentionGroup {
	return apiv1.MentionGroup{
		Source:      descriptor.Source,
		Provider:    descriptor.Provider,
		Kind:        descriptor.Kind,
		DisplayName: descriptor.DisplayName,
		KindLabel:   descriptor.KindLabel,
		Status:      apiv1.MentionStatusOK,
		Results:     []apiv1.EntityReference{},
	}
}

func referenceFromCandidate(descriptor ProviderDescriptor, candidate Candidate) apiv1.EntityReference {
	return apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef(descriptor.Provider, descriptor.Kind, candidate.Scope, candidate.ID),
		Provider: descriptor.Provider,
		Kind:     descriptor.Kind,
		ID:       candidate.ID,
		Key:      candidate.Key,
		Title:    candidate.Title,
		URL:      candidate.URL,
		Scope:    candidate.Scope,
	}
}

func canonicalRef(provider, kind, scope, id string) string {
	return entityrefs.CanonicalRef(provider, kind, scope, id)
}
