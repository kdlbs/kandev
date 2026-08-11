// Package pluginsdk is the author-facing SDK for kandev plugin backends, and
// the shared wiring kandev's own runtime manager imports to spawn and talk
// to those plugins. See docs/plans/plugins/GRPC-CONTRACT.md §4 for the
// frozen public surface this package implements.
//
// types.go defines the Go-native mirrors of the kandev.plugin.v1 proto
// messages (using map[string]any in place of google.protobuf.Struct) plus
// the proto<->Go conversion helpers used by every other file in this
// package. Authors and kandev's runtime manager only ever see these
// Go-native types; proto types from
// github.com/kandev/kandev/proto/kandev/plugin/v1 never leak past the
// package boundary.
package pluginsdk

import (
	"fmt"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// Event is the Go-native mirror of kandev.plugin.v1.Event, delivered to a
// Plugin's OnEvent method.
type Event struct {
	EventID     string
	EventType   string
	OccurredAt  string
	WorkspaceID string
	Payload     map[string]any
}

func (e *Event) toProto() (*pluginv1.Event, error) {
	payload, err := mapToStruct(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: event payload: %w", err)
	}
	return &pluginv1.Event{
		EventId:     e.EventID,
		EventType:   e.EventType,
		OccurredAt:  e.OccurredAt,
		WorkspaceId: e.WorkspaceID,
		Payload:     payload,
	}, nil
}

func eventFromProto(p *pluginv1.Event) (*Event, error) {
	payload, err := structToMap(p.GetPayload())
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: event payload: %w", err)
	}
	return &Event{
		EventID:     p.GetEventId(),
		EventType:   p.GetEventType(),
		OccurredAt:  p.GetOccurredAt(),
		WorkspaceID: p.GetWorkspaceId(),
		Payload:     payload,
	}, nil
}

// WebhookRequest is the Go-native mirror of kandev.plugin.v1.WebhookRequest,
// delivered to a Plugin's HandleWebhook method. Unlike Event, it has no
// Struct-typed fields so conversion cannot fail.
type WebhookRequest struct {
	WebhookKey string
	Method     string
	Path       string
	Query      string
	Headers    map[string]string
	Body       []byte
}

func (r *WebhookRequest) toProto() *pluginv1.WebhookRequest {
	return &pluginv1.WebhookRequest{
		WebhookKey: r.WebhookKey,
		Method:     r.Method,
		Path:       r.Path,
		Query:      r.Query,
		Headers:    r.Headers,
		Body:       r.Body,
	}
}

func webhookRequestFromProto(p *pluginv1.WebhookRequest) *WebhookRequest {
	return &WebhookRequest{
		WebhookKey: p.GetWebhookKey(),
		Method:     p.GetMethod(),
		Path:       p.GetPath(),
		Query:      p.GetQuery(),
		Headers:    p.GetHeaders(),
		Body:       p.GetBody(),
	}
}

// WebhookResponse is the Go-native mirror of kandev.plugin.v1.WebhookResponse,
// returned by a Plugin's HandleWebhook method.
type WebhookResponse struct {
	Status  int32
	Headers map[string]string
	Body    []byte
}

func (r *WebhookResponse) toProto() *pluginv1.WebhookResponse {
	return &pluginv1.WebhookResponse{
		Status:  r.Status,
		Headers: r.Headers,
		Body:    r.Body,
	}
}

func webhookResponseFromProto(p *pluginv1.WebhookResponse) *WebhookResponse {
	return &WebhookResponse{
		Status:  p.GetStatus(),
		Headers: p.GetHeaders(),
		Body:    p.GetBody(),
	}
}

// VerifiedActionContext is derived by Kandev after authenticating and
// authorizing an action request. Plugins must treat Body as untrusted input
// and use this context for all authority decisions.
type VerifiedActionContext struct {
	ActorID      string
	WorkspaceID  string
	TaskID       string
	RepositoryID string
	SessionID    string
	HeadBranch   string
}

func (c VerifiedActionContext) toProto() *pluginv1.VerifiedActionContext {
	return &pluginv1.VerifiedActionContext{
		ActorId:      c.ActorID,
		WorkspaceId:  c.WorkspaceID,
		TaskId:       c.TaskID,
		RepositoryId: c.RepositoryID,
		SessionId:    c.SessionID,
		HeadBranch:   c.HeadBranch,
	}
}

func verifiedActionContextFromProto(p *pluginv1.VerifiedActionContext) VerifiedActionContext {
	if p == nil {
		return VerifiedActionContext{}
	}
	return VerifiedActionContext{
		ActorID:      p.GetActorId(),
		WorkspaceID:  p.GetWorkspaceId(),
		TaskID:       p.GetTaskId(),
		RepositoryID: p.GetRepositoryId(),
		SessionID:    p.GetSessionId(),
		HeadBranch:   p.GetHeadBranch(),
	}
}

// PluginActionRequest is the Go-native mirror of PluginActionRequest.
type PluginActionRequest struct {
	ActionKey string
	Context   VerifiedActionContext
	Body      []byte
}

func (r *PluginActionRequest) toProto() *pluginv1.PluginActionRequest {
	return &pluginv1.PluginActionRequest{ActionKey: r.ActionKey, Context: r.Context.toProto(), Body: r.Body}
}

func pluginActionRequestFromProto(p *pluginv1.PluginActionRequest) *PluginActionRequest {
	if p == nil {
		return &PluginActionRequest{}
	}
	return &PluginActionRequest{ActionKey: p.GetActionKey(), Context: verifiedActionContextFromProto(p.GetContext()), Body: p.GetBody()}
}

// PluginActionResponse is the Go-native mirror of PluginActionResponse.
// Kandev filters Headers before returning them to the browser.
type PluginActionResponse struct {
	Body    []byte
	Headers map[string]string
	// Status is a final HTTP status in [200, 599]. Zero means 200 for
	// compatibility with plugins built before domain responses were added.
	Status int
}

func (r *PluginActionResponse) toProto() *pluginv1.PluginActionResponse {
	return &pluginv1.PluginActionResponse{Body: r.Body, Headers: r.Headers, Status: int32(r.Status)}
}

func pluginActionResponseFromProto(p *pluginv1.PluginActionResponse) *PluginActionResponse {
	if p == nil {
		return &PluginActionResponse{}
	}
	return &PluginActionResponse{Body: p.GetBody(), Headers: p.GetHeaders(), Status: int(p.GetStatus())}
}

// SearchEntityReferencesRequest is a host-verified workspace search request.
type SearchEntityReferencesRequest struct {
	Source      string
	WorkspaceID string
	Query       string
	Limit       int32
}

func (r *SearchEntityReferencesRequest) toProto() *pluginv1.SearchEntityReferencesRequest {
	return &pluginv1.SearchEntityReferencesRequest{Source: r.Source, WorkspaceId: r.WorkspaceID, Query: r.Query, Limit: r.Limit}
}

func searchEntityReferencesRequestFromProto(p *pluginv1.SearchEntityReferencesRequest) *SearchEntityReferencesRequest {
	if p == nil {
		return &SearchEntityReferencesRequest{}
	}
	return &SearchEntityReferencesRequest{Source: p.GetSource(), WorkspaceID: p.GetWorkspaceId(), Query: p.GetQuery(), Limit: p.GetLimit()}
}

// EntityReferenceCandidate is untrusted plugin search output. Kandev adds
// manifest-owned descriptor identity before it reaches a composer result.
type EntityReferenceCandidate struct {
	ProviderLocalID string
	Title           string
	URL             string
	Attributes      map[string]any
}

func (c EntityReferenceCandidate) toProto() (*pluginv1.EntityReferenceCandidate, error) {
	attributes, err := mapToStruct(c.Attributes)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: reference candidate attributes: %w", err)
	}
	return &pluginv1.EntityReferenceCandidate{ProviderLocalId: c.ProviderLocalID, Title: c.Title, Url: c.URL, Attributes: attributes}, nil
}

func entityReferenceCandidateFromProto(p *pluginv1.EntityReferenceCandidate) (EntityReferenceCandidate, error) {
	if p == nil {
		return EntityReferenceCandidate{}, nil
	}
	attributes, err := structToMap(p.GetAttributes())
	if err != nil {
		return EntityReferenceCandidate{}, fmt.Errorf("pluginsdk: reference candidate attributes: %w", err)
	}
	return EntityReferenceCandidate{ProviderLocalID: p.GetProviderLocalId(), Title: p.GetTitle(), URL: p.GetUrl(), Attributes: attributes}, nil
}

func entityReferenceCandidatesToProto(candidates []EntityReferenceCandidate) ([]*pluginv1.EntityReferenceCandidate, error) {
	if candidates == nil {
		return nil, nil
	}
	out := make([]*pluginv1.EntityReferenceCandidate, len(candidates))
	for i, candidate := range candidates {
		converted, err := candidate.toProto()
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

func entityReferenceCandidatesFromProto(candidates []*pluginv1.EntityReferenceCandidate) ([]EntityReferenceCandidate, error) {
	if candidates == nil {
		return nil, nil
	}
	out := make([]EntityReferenceCandidate, len(candidates))
	for i, candidate := range candidates {
		converted, err := entityReferenceCandidateFromProto(candidate)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

// SearchEntityReferencesResponse is the Go-native search response.
type SearchEntityReferencesResponse struct{ Candidates []EntityReferenceCandidate }

func (r *SearchEntityReferencesResponse) toProto() (*pluginv1.SearchEntityReferencesResponse, error) {
	candidates, err := entityReferenceCandidatesToProto(r.Candidates)
	if err != nil {
		return nil, err
	}
	return &pluginv1.SearchEntityReferencesResponse{Candidates: candidates}, nil
}

func searchEntityReferencesResponseFromProto(p *pluginv1.SearchEntityReferencesResponse) (*SearchEntityReferencesResponse, error) {
	if p == nil {
		return &SearchEntityReferencesResponse{}, nil
	}
	candidates, err := entityReferenceCandidatesFromProto(p.GetCandidates())
	if err != nil {
		return nil, err
	}
	return &SearchEntityReferencesResponse{Candidates: candidates}, nil
}

// AuthorizeEntityReferenceRequest asks a live plugin to approve a canonical
// reference for search or final message submission.
type AuthorizeEntityReferenceRequest struct {
	Source      string
	WorkspaceID string
	Purpose     string
	Reference   map[string]any
}

func (r *AuthorizeEntityReferenceRequest) toProto() (*pluginv1.AuthorizeEntityReferenceRequest, error) {
	reference, err := mapToStruct(r.Reference)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: entity reference: %w", err)
	}
	return &pluginv1.AuthorizeEntityReferenceRequest{Source: r.Source, WorkspaceId: r.WorkspaceID, Purpose: r.Purpose, Reference: reference}, nil
}

func authorizeEntityReferenceRequestFromProto(p *pluginv1.AuthorizeEntityReferenceRequest) (*AuthorizeEntityReferenceRequest, error) {
	if p == nil {
		return &AuthorizeEntityReferenceRequest{}, nil
	}
	reference, err := structToMap(p.GetReference())
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: entity reference: %w", err)
	}
	return &AuthorizeEntityReferenceRequest{Source: p.GetSource(), WorkspaceID: p.GetWorkspaceId(), Purpose: p.GetPurpose(), Reference: reference}, nil
}

type AuthorizeEntityReferenceResponse struct {
	Allowed bool
	Reason  string
}

func (r *AuthorizeEntityReferenceResponse) toProto() *pluginv1.AuthorizeEntityReferenceResponse {
	return &pluginv1.AuthorizeEntityReferenceResponse{Allowed: r.Allowed, Reason: r.Reason}
}

func authorizeEntityReferenceResponseFromProto(p *pluginv1.AuthorizeEntityReferenceResponse) *AuthorizeEntityReferenceResponse {
	if p == nil {
		return &AuthorizeEntityReferenceResponse{}
	}
	return &AuthorizeEntityReferenceResponse{Allowed: p.GetAllowed(), Reason: p.GetReason()}
}

// ResolveGitCredentialRequest contains exact host-verified lease scope.
type ResolveGitCredentialRequest struct {
	ProviderID   string
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
	Host         string
	Path         string
}

func (r *ResolveGitCredentialRequest) toProto() *pluginv1.ResolveGitCredentialRequest {
	return &pluginv1.ResolveGitCredentialRequest{ProviderId: r.ProviderID, WorkspaceId: r.WorkspaceID, TaskId: r.TaskID, SessionId: r.SessionID, RepositoryId: r.RepositoryID, Host: r.Host, Path: r.Path}
}

func resolveGitCredentialRequestFromProto(p *pluginv1.ResolveGitCredentialRequest) *ResolveGitCredentialRequest {
	if p == nil {
		return &ResolveGitCredentialRequest{}
	}
	return &ResolveGitCredentialRequest{ProviderID: p.GetProviderId(), WorkspaceID: p.GetWorkspaceId(), TaskID: p.GetTaskId(), SessionID: p.GetSessionId(), RepositoryID: p.GetRepositoryId(), Host: p.GetHost(), Path: p.GetPath()}
}

// ResolveGitCredentialResponse is transient authentication material. Hosts
// must not log or persist Secret.
type ResolveGitCredentialResponse struct {
	Username  string
	Secret    string
	ExpiresAt string
}

func (r *ResolveGitCredentialResponse) toProto() *pluginv1.ResolveGitCredentialResponse {
	return &pluginv1.ResolveGitCredentialResponse{Username: r.Username, Secret: r.Secret, ExpiresAt: r.ExpiresAt}
}

func resolveGitCredentialResponseFromProto(p *pluginv1.ResolveGitCredentialResponse) *ResolveGitCredentialResponse {
	if p == nil {
		return &ResolveGitCredentialResponse{}
	}
	return &ResolveGitCredentialResponse{Username: p.GetUsername(), Secret: p.GetSecret(), ExpiresAt: p.GetExpiresAt()}
}

// GitCredentialBindingRequest contains the exact host-verified lease scope
// for a non-secret credential revision lookup.
type GitCredentialBindingRequest struct {
	ProviderID   string
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
	Host         string
	Path         string
}

func (r *GitCredentialBindingRequest) toProto() *pluginv1.GitCredentialBindingRequest {
	return &pluginv1.GitCredentialBindingRequest{
		ProviderId: r.ProviderID, WorkspaceId: r.WorkspaceID, TaskId: r.TaskID, SessionId: r.SessionID,
		RepositoryId: r.RepositoryID, Host: r.Host, Path: r.Path,
	}
}

func gitCredentialBindingRequestFromProto(p *pluginv1.GitCredentialBindingRequest) *GitCredentialBindingRequest {
	if p == nil {
		return &GitCredentialBindingRequest{}
	}
	return &GitCredentialBindingRequest{
		ProviderID: p.GetProviderId(), WorkspaceID: p.GetWorkspaceId(), TaskID: p.GetTaskId(), SessionID: p.GetSessionId(),
		RepositoryID: p.GetRepositoryId(), Host: p.GetHost(), Path: p.GetPath(),
	}
}

// GitCredentialBindingResponse returns an opaque, non-secret revision for a
// credential scope. Hosts must treat an empty response as a revoked binding.
type GitCredentialBindingResponse struct{ Binding string }

func (r *GitCredentialBindingResponse) toProto() *pluginv1.GitCredentialBindingResponse {
	return &pluginv1.GitCredentialBindingResponse{Binding: r.Binding}
}

func gitCredentialBindingResponseFromProto(p *pluginv1.GitCredentialBindingResponse) *GitCredentialBindingResponse {
	if p == nil {
		return &GitCredentialBindingResponse{}
	}
	return &GitCredentialBindingResponse{Binding: p.GetBinding()}
}

// StateEntry is the Go-native mirror of kandev.plugin.v1.StateEntry, as
// returned by Host.ListState.
type StateEntry struct {
	Key       string
	Value     map[string]any
	UpdatedAt string
}

func (e *StateEntry) toProto() (*pluginv1.StateEntry, error) {
	value, err := mapToStruct(e.Value)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: state entry value: %w", err)
	}
	return &pluginv1.StateEntry{
		Key:       e.Key,
		Value:     value,
		UpdatedAt: e.UpdatedAt,
	}, nil
}

func stateEntryFromProto(p *pluginv1.StateEntry) (*StateEntry, error) {
	value, err := structToMap(p.GetValue())
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: state entry value: %w", err)
	}
	return &StateEntry{
		Key:       p.GetKey(),
		Value:     value,
		UpdatedAt: p.GetUpdatedAt(),
	}, nil
}

func stateEntriesFromProto(entries []*pluginv1.StateEntry) ([]StateEntry, error) {
	if entries == nil {
		return nil, nil
	}
	out := make([]StateEntry, len(entries))
	for i, e := range entries {
		converted, err := stateEntryFromProto(e)
		if err != nil {
			return nil, err
		}
		out[i] = *converted
	}
	return out, nil
}

// mapToStruct converts a Go-native map to a google.protobuf.Struct. A nil
// map converts to a nil Struct (distinguishing "no payload" from "empty
// payload" across the wire).
func mapToStruct(m map[string]any) (*structpb.Struct, error) {
	if m == nil {
		return nil, nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: convert map to struct: %w", err)
	}
	return s, nil
}

// structToMap converts a google.protobuf.Struct to a Go-native map. A nil
// Struct converts to a nil map.
func structToMap(s *structpb.Struct) (map[string]any, error) {
	if s == nil {
		return nil, nil
	}
	return s.AsMap(), nil
}
