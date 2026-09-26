package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/mentions"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	referenceAuthorizationTimeout = 1500 * time.Millisecond
	entityReferenceProviderField  = "provider"
)

// MentionSourceClient is the live plugin-service surface needed to materialize
// manifest-declared composer sources.
type MentionSourceClient interface {
	List() []*store.Record
	SearchEntityReferences(
		context.Context, string, pluginDispatchGeneration, *pluginsdk.SearchEntityReferencesRequest,
	) (*pluginsdk.SearchEntityReferencesResponse, error)
	AuthorizeEntityReference(
		context.Context, string, pluginDispatchGeneration, *pluginsdk.AuthorizeEntityReferenceRequest,
	) (*pluginsdk.AuthorizeEntityReferenceResponse, error)
}

// MentionSourceBridge materializes active manifest-owned reference sources
// into mentions.Registry. It never accepts plugin-provided descriptor identity.
type MentionSourceBridge struct {
	mu           sync.Mutex
	client       MentionSourceClient
	registry     *mentions.Registry
	owners       map[string]struct{}
	fingerprints map[string]string
}

func NewMentionSourceBridge(client MentionSourceClient) *MentionSourceBridge {
	return &MentionSourceBridge{
		client: client, owners: make(map[string]struct{}), fingerprints: make(map[string]string),
	}
}

// Descriptor and Search make the bridge transport-compatible with the
// existing bootstrap list. backendapp recognizes SourceRegistrar first, so
// this placeholder is never itself a searchable source.
func (*MentionSourceBridge) Descriptor() mentions.ProviderDescriptor {
	return mentions.ProviderDescriptor{}
}

func (*MentionSourceBridge) Search(context.Context, mentions.SearchRequest) ([]mentions.Candidate, error) {
	return nil, errors.New("plugin mention source bridge is not directly searchable")
}

// RegisterMentionSources installs this bridge as a live registry refresher
// and materializes the current active-plugin snapshot.
func (b *MentionSourceBridge) RegisterMentionSources(registry *mentions.Registry) error {
	if b == nil || registry == nil {
		return errors.New("plugin mention source registry is unavailable")
	}
	b.mu.Lock()
	b.registry = registry
	b.mu.Unlock()
	registry.RegisterSourceRefresher(b)
	return b.Sync()
}

// RefreshMentionSources satisfies mentions.SourceRefresher. Registry reads
// cannot surface refresh errors, so Sync keeps the prior owner set intact on
// a collision and authorization remains fail-closed.
func (b *MentionSourceBridge) RefreshMentionSources() {
	_ = b.Sync()
}

// Sync replaces every plugin-owned source from the current active manifest
// snapshot. Sources disappear when their plugin is disabled or uninstalled.
func (b *MentionSourceBridge) Sync() error {
	if b == nil || b.client == nil {
		return errors.New("plugin mention source client is unavailable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.registry == nil {
		return errors.New("plugin mention source registry is unavailable")
	}
	return b.syncLocked(activeMentionSourceProviders(b.client, b.client.List()))
}

func (b *MentionSourceBridge) syncLocked(next map[string][]mentions.MentionProvider) error {
	var errs []error
	// Release departed owners first. Otherwise a successor whose ID sorts
	// before the disabled predecessor collides on the first refresh and the
	// newly active source stays invisible until a second request.
	for _, owner := range departedMentionSourceOwners(b.owners, next) {
		if err := b.registry.ReplaceOwner(owner); err != nil {
			errs = append(errs, fmt.Errorf("plugin %q reference sources: %w", owner, err))
			continue
		}
		delete(b.owners, owner)
		delete(b.fingerprints, owner)
	}
	for _, owner := range sortedMentionSourceOwners(next) {
		providers := next[owner]
		fingerprint := mentionSourceFingerprint(providers)
		if _, exists := b.owners[owner]; exists && b.fingerprints[owner] == fingerprint {
			continue
		}
		if err := b.registry.ReplaceOwner(owner, providers...); err != nil {
			errs = append(errs, fmt.Errorf("plugin %q reference sources: %w", owner, err))
			continue
		}
		b.owners[owner] = struct{}{}
		b.fingerprints[owner] = fingerprint
	}
	return errors.Join(errs...)
}

func activeMentionSourceProviders(client MentionSourceClient, records []*store.Record) map[string][]mentions.MentionProvider {
	providers := make(map[string][]mentions.MentionProvider)
	for _, record := range records {
		if record == nil || record.Status != StatusActive || len(record.ReferenceSources) == 0 {
			continue
		}
		for _, source := range record.ReferenceSources {
			providers[record.ID] = append(providers[record.ID], pluginMentionProvider{
				client:     client,
				pluginID:   record.ID,
				generation: dispatchGeneration(record),
				descriptor: source,
			})
		}
	}
	return providers
}

func departedMentionSourceOwners(
	current map[string]struct{}, next map[string][]mentions.MentionProvider,
) []string {
	owners := make([]string, 0, len(current))
	for owner := range current {
		if _, retained := next[owner]; retained {
			continue
		}
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	return owners
}

func sortedMentionSourceOwners(next map[string][]mentions.MentionProvider) []string {
	owners := make([]string, 0, len(next))
	for owner := range next {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	return owners
}

func mentionSourceFingerprint(providers []mentions.MentionProvider) string {
	var result strings.Builder
	for _, provider := range providers {
		descriptor := provider.Descriptor()
		generation := pluginDispatchGeneration{}
		if pluginProvider, ok := provider.(pluginMentionProvider); ok {
			generation = pluginProvider.generation
		}
		_, _ = fmt.Fprintf(
			&result,
			"%q\x00%q\x00%q\x00%q\x00%q\x00%d\x00%q\x00%q\x00%d\x00",
			descriptor.Source, descriptor.Provider, descriptor.Kind,
			descriptor.DisplayName, descriptor.KindLabel, descriptor.Order,
			generation.version, generation.installPath, generation.installedAt.UnixNano(),
		)
	}
	return result.String()
}

type pluginMentionProvider struct {
	client     MentionSourceClient
	pluginID   string
	generation pluginDispatchGeneration
	descriptor manifest.ReferenceSource
}

func (p pluginMentionProvider) Descriptor() mentions.ProviderDescriptor {
	return mentions.ProviderDescriptor{
		Source: p.descriptor.Source, Provider: p.descriptor.Provider, Kind: p.descriptor.Kind,
		DisplayName: p.descriptor.DisplayName, KindLabel: p.descriptor.KindLabel, Order: p.descriptor.Order,
	}
}

func (p pluginMentionProvider) Search(
	ctx context.Context, request mentions.SearchRequest,
) ([]mentions.Candidate, error) {
	response, err := p.client.SearchEntityReferences(ctx, p.pluginID, p.generation, &pluginsdk.SearchEntityReferencesRequest{
		Source: p.descriptor.Source, WorkspaceID: request.WorkspaceID, Query: request.Query, Limit: int32(request.Limit),
	})
	if err != nil || response == nil {
		return nil, err
	}
	return hostCandidates(response.Candidates, request.WorkspaceID), nil
}

func hostCandidates(candidates []pluginsdk.EntityReferenceCandidate, workspaceID string) []mentions.Candidate {
	result := make([]mentions.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, mentions.Candidate{
			ID: candidate.ProviderLocalID, Key: candidate.ProviderLocalID, Title: candidate.Title,
			URL: candidate.URL, Scope: workspaceID,
		})
	}
	return result
}

func (p pluginMentionProvider) AuthorizeReference(
	ctx context.Context, request mentions.ReferenceAuthorizationRequest,
) error {
	if !p.matchesReference(request) {
		return mentions.ErrReferenceUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, referenceAuthorizationTimeout)
	defer cancel()
	response, err := p.client.AuthorizeEntityReference(ctx, p.pluginID, p.generation, &pluginsdk.AuthorizeEntityReferenceRequest{
		Source: p.descriptor.Source, WorkspaceID: request.WorkspaceID, Purpose: string(request.Purpose),
		Reference: pluginEntityReference(request.Reference),
	})
	if err != nil || response == nil || !response.Allowed {
		return mentions.ErrReferenceUnauthorized
	}
	return nil
}

func (p pluginMentionProvider) matchesReference(request mentions.ReferenceAuthorizationRequest) bool {
	return request.Reference.Provider == p.descriptor.Provider &&
		request.Reference.Kind == p.descriptor.Kind && request.Reference.Scope == request.WorkspaceID
}

func pluginEntityReference(reference apiv1.EntityReference) map[string]any {
	return map[string]any{
		"version":                    reference.Version,
		"ref":                        reference.Ref,
		entityReferenceProviderField: reference.Provider,
		"kind":                       reference.Kind,
		"id":                         reference.ID,
		"key":                        reference.Key,
		"title":                      reference.Title,
		"url":                        reference.URL,
		"scope":                      reference.Scope,
	}
}

// SearchEntityReferences routes one validated manifest source to its live
// plugin process. The bridge also checks active status before every RPC so a
// disable race cannot return or authorize a reference.
func (s *Service) SearchEntityReferences(
	ctx context.Context, id string, expected pluginDispatchGeneration, request *pluginsdk.SearchEntityReferencesRequest,
) (*pluginsdk.SearchEntityReferencesResponse, error) {
	if request == nil {
		return nil, errors.New("plugins: missing entity reference request")
	}
	_, release, err := s.beginPluginDispatch(id, expected)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.requireActiveReferenceSource(id, request.Source); err != nil {
		return nil, err
	}
	remote, ok := s.pluginRemote(id)
	if !ok || remote == nil {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	return remote.SearchEntityReferences(ctx, request)
}

// AuthorizeEntityReference routes submit-time authorization for one validated
// manifest source to its live plugin process.
func (s *Service) AuthorizeEntityReference(
	ctx context.Context, id string, expected pluginDispatchGeneration, request *pluginsdk.AuthorizeEntityReferenceRequest,
) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
	if request == nil {
		return nil, errors.New("plugins: missing entity reference request")
	}
	_, release, err := s.beginPluginDispatch(id, expected)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.requireActiveReferenceSource(id, request.Source); err != nil {
		return nil, err
	}
	remote, ok := s.pluginRemote(id)
	if !ok || remote == nil {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	return remote.AuthorizeEntityReference(ctx, request)
}

func (s *Service) requireActiveReferenceSource(id, sourceName string) error {
	record, err := s.Get(id)
	if err != nil {
		return err
	}
	if record.Status != StatusActive {
		return fmt.Errorf("plugins: plugin %q is not active", id)
	}
	for _, source := range record.ReferenceSources {
		if source.Source == sourceName {
			return nil
		}
	}
	return fmt.Errorf("plugins: plugin %q does not declare reference source %q", id, sourceName)
}
