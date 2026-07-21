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

	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type registeredProvider struct {
	descriptor ProviderDescriptor
	provider   MentionProvider
	authorizer ReferenceAuthorizer
}

// Registry stores provider descriptors independently of their implementation type.
type Registry struct {
	mu                   sync.RWMutex
	providers            []registeredProvider
	sources              map[string]struct{}
	providerKinds        map[string]struct{}
	referenceAuthorizers map[string]ReferenceAuthorizer
}

func NewRegistry() *Registry {
	return &Registry{
		sources:              make(map[string]struct{}),
		providerKinds:        make(map[string]struct{}),
		referenceAuthorizers: make(map[string]ReferenceAuthorizer),
	}
}

func (r *Registry) Register(provider MentionProvider) error {
	if provider == nil {
		return fmt.Errorf("%w: provider is nil", ErrInvalidDescriptor)
	}
	descriptor, err := normalizeDescriptor(provider.Descriptor())
	if err != nil {
		return err
	}
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok || authorizer == nil {
		return ErrMissingAuthorizer
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sources == nil {
		r.sources = make(map[string]struct{})
	}
	if r.providerKinds == nil {
		r.providerKinds = make(map[string]struct{})
	}
	if r.referenceAuthorizers == nil {
		r.referenceAuthorizers = make(map[string]ReferenceAuthorizer)
	}
	if _, exists := r.sources[descriptor.Source]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateSource, descriptor.Source)
	}
	providerKey := referenceProviderKey(descriptor.Provider, descriptor.Kind)
	if _, exists := r.providerKinds[providerKey]; exists {
		return fmt.Errorf("%w: %s/%s", ErrDuplicateProvider, descriptor.Provider, descriptor.Kind)
	}
	r.providers = append(r.providers, registeredProvider{
		descriptor: descriptor,
		provider:   provider,
		authorizer: authorizer,
	})
	r.sources[descriptor.Source] = struct{}{}
	r.providerKinds[providerKey] = struct{}{}
	r.referenceAuthorizers[providerKey] = authorizer
	return nil
}

func referenceProviderKey(provider, kind string) string {
	return provider + "\x00" + kind
}

// AuthorizeReference dispatches to the provider registered for the normalized identity.
func (r *Registry) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
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
		group := searchProviderWithinContext(providerCtx, registered, request)
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
) apiv1.MentionGroup {
	group := groupFromDescriptor(registered.descriptor)
	candidates, err := registered.provider.Search(ctx, request)
	if err != nil {
		group.Status = classifyProviderError(err)
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
		candidate, ok := normalizeCandidate(registered.descriptor, candidate)
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

func normalizeCandidate(descriptor ProviderDescriptor, candidate Candidate) (Candidate, bool) {
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
	if !safeMentionURL(descriptor, candidate.ID, candidate.URL) {
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

func safeMentionURL(descriptor ProviderDescriptor, resourceID, raw string) bool {
	if raw == "" || !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 2048 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return false
	}
	if descriptor.Provider == mentionProviderKandev && descriptor.Kind == mentionKindTask {
		return raw == "/t/"+url.PathEscape(resourceID)
	}
	if parsed.Scheme == "" && parsed.Host == "" {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func normalizeRequest(request SearchRequest) (SearchRequest, error) {
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	request.Query = strings.TrimSpace(request.Query)
	request.ExcludeTaskID = strings.TrimSpace(request.ExcludeTaskID)
	if !validIdentity(request.WorkspaceID) {
		return SearchRequest{}, fmt.Errorf("%w: workspace ID is required", ErrInvalidRequest)
	}
	if request.ExcludeTaskID != "" && !validIdentity(request.ExcludeTaskID) {
		return SearchRequest{}, fmt.Errorf("%w: excluded task ID is invalid", ErrInvalidRequest)
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
