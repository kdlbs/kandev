// host.go implements the Host side of the kandev.plugin.v1.Host service
// (§3 of docs/plans/plugins/GRPC-CONTRACT.md) in both directions:
//
//   - grpcHostClient: used inside the plugin subprocess. Wraps a
//     pluginv1.HostClient dialed over the go-plugin broker (see serve.go)
//     and satisfies the Go-native Host interface that Serve injects into
//     the author's Plugin.
//   - grpcHostServer: used inside kandev. Wraps kandev's own Go-native Host
//     implementation (state store, secrets, event bus) and satisfies the
//     generated pluginv1.HostServer interface so it can be registered on
//     the broker-served grpc.Server that GRPCPlugin.GRPCClient spins up
//     (see serve.go's "Host injection" section).
//
// Both directions share the same Go-native Host interface and the same
// proto conversion helpers in types.go, so kandev's runtime manager
// implements Host exactly once and gets both the client and server wiring
// for free via GRPCPlugin.
package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Host is the set of operations kandev exposes back to a running plugin,
// per §3's Host service. On the plugin side, Serve injects an
// SDK-provided implementation that proxies these calls to kandev over the
// go-plugin broker. On the kandev side, the runtime manager provides its
// own Go-native implementation of this same interface (backed by the real
// state store / secrets / event bus) and hands it to GRPCPlugin.Host; the
// SDK wraps it into the generated pluginv1.HostServer for registration.
type Host interface {
	// GetState looks up a single state entry. found is false, err is nil
	// when the key does not exist.
	GetState(ctx context.Context, scope, scopeID, key string) (value map[string]any, found bool, err error)

	// SetState upserts a single state entry.
	SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error

	// DeleteState removes a single state entry. Deleting a missing key is
	// not an error.
	DeleteState(ctx context.Context, scope, scopeID, key string) error

	// ListState returns every state entry for a scope/scopeID pair.
	ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error)

	// RevealSecret resolves a secret reference to its cleartext value.
	RevealSecret(ctx context.Context, ref string) (string, error)

	// EmitEvent publishes a plugin-originated event onto kandev's bus.
	EmitEvent(ctx context.Context, name string, payload map[string]any) error

	// Tasks returns the reader for the Host data API's task RPCs
	// (capability api_read:tasks per ADR 0043).
	Tasks() TaskReader

	// Sessions returns the reader for the Host data API's session and
	// session-code-stats RPCs (capability api_read:sessions).
	Sessions() SessionReader

	// Workspaces returns the reader for the Host data API's workspace RPCs
	// (capability api_read:workspaces).
	Workspaces() WorkspaceReader

	// Workflows returns the reader for the Host data API's workflow and
	// workflow-step RPCs (capability api_read:workflows).
	Workflows() WorkflowReader

	// AgentProfiles returns the reader for the Host data API's agent
	// profile RPCs (capability api_read:agent_profiles).
	AgentProfiles() AgentProfileReader

	// Repositories returns the reader for the Host data API's repository
	// RPCs (capability api_read:repositories).
	Repositories() RepositoryReader
}

// TaskReader is the read-only accessor behind Host.Tasks(), mirroring the
// Host data API's ListTasks/GetTask RPCs (ADR 0043). Write methods
// (CreateTask/UpdateTask) are deferred to a later phase and intentionally
// not part of this interface yet.
type TaskReader interface {
	// List returns tasks matching filter, newest page first per page.
	List(ctx context.Context, filter TaskFilter, page Page) ([]Task, *PageInfo, error)

	// Get returns a single task by id.
	Get(ctx context.Context, id string) (*Task, error)
}

// SessionReader is the read-only accessor behind Host.Sessions(), mirroring
// the Host data API's ListSessions/ListSessionCodeStats RPCs.
type SessionReader interface {
	// List returns sessions matching filter.
	List(ctx context.Context, filter SessionFilter, page Page) ([]Session, *PageInfo, error)

	// CodeStats returns computed per-session code-change stats matching
	// filter. SessionCodeStats is a stable, computed shape — never raw
	// commit/snapshot rows.
	CodeStats(ctx context.Context, filter SessionFilter, page Page) ([]SessionCodeStats, *PageInfo, error)
}

// WorkspaceReader is the read-only accessor behind Host.Workspaces(),
// mirroring the Host data API's ListWorkspaces RPC.
type WorkspaceReader interface {
	List(ctx context.Context, page Page) ([]Workspace, *PageInfo, error)
}

// WorkflowReader is the read-only accessor behind Host.Workflows(),
// mirroring the Host data API's ListWorkflows/ListWorkflowSteps RPCs.
type WorkflowReader interface {
	// List returns workflows for workspaceID.
	List(ctx context.Context, workspaceID string, page Page) ([]Workflow, *PageInfo, error)

	// ListSteps returns the steps for workflowID, in position order.
	ListSteps(ctx context.Context, workflowID string) ([]WorkflowStep, error)
}

// AgentProfileReader is the read-only accessor behind Host.AgentProfiles(),
// mirroring the Host data API's ListAgentProfiles RPC.
type AgentProfileReader interface {
	List(ctx context.Context, page Page) ([]AgentProfile, *PageInfo, error)
}

// RepositoryReader is the read-only accessor behind Host.Repositories(),
// mirroring the Host data API's ListRepositories RPC.
type RepositoryReader interface {
	// List returns repositories for workspaceID.
	List(ctx context.Context, workspaceID string, page Page) ([]Repository, *PageInfo, error)
}

// newHostClient wraps a *grpc.ClientConn (dialed over the go-plugin broker)
// as a Go-native Host implementation.
func newHostClient(conn *grpc.ClientConn) Host {
	return &grpcHostClient{client: pluginv1.NewHostClient(conn)}
}

type grpcHostClient struct {
	client pluginv1.HostClient
}

func (h *grpcHostClient) GetState(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
	resp, err := h.client.GetState(ctx, &pluginv1.GetStateRequest{Scope: scope, ScopeId: scopeID, Key: key})
	if err != nil {
		return nil, false, err
	}
	if !resp.GetFound() {
		return nil, false, nil
	}
	value, err := structToMap(resp.GetValue())
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (h *grpcHostClient) SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error {
	protoValue, err := mapToStruct(value)
	if err != nil {
		return err
	}
	_, err = h.client.SetState(ctx, &pluginv1.SetStateRequest{Scope: scope, ScopeId: scopeID, Key: key, Value: protoValue})
	return err
}

func (h *grpcHostClient) DeleteState(ctx context.Context, scope, scopeID, key string) error {
	_, err := h.client.DeleteState(ctx, &pluginv1.DeleteStateRequest{Scope: scope, ScopeId: scopeID, Key: key})
	return err
}

func (h *grpcHostClient) ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error) {
	resp, err := h.client.ListState(ctx, &pluginv1.ListStateRequest{Scope: scope, ScopeId: scopeID})
	if err != nil {
		return nil, err
	}
	return stateEntriesFromProto(resp.GetEntries())
}

func (h *grpcHostClient) RevealSecret(ctx context.Context, ref string) (string, error) {
	resp, err := h.client.RevealSecret(ctx, &pluginv1.RevealSecretRequest{Ref: ref})
	if err != nil {
		return "", err
	}
	return resp.GetValue(), nil
}

func (h *grpcHostClient) EmitEvent(ctx context.Context, name string, payload map[string]any) error {
	protoPayload, err := mapToStruct(payload)
	if err != nil {
		return err
	}
	_, err = h.client.EmitEvent(ctx, &pluginv1.EmitEventRequest{EventName: name, Payload: protoPayload})
	return err
}

func (h *grpcHostClient) Tasks() TaskReader { return grpcTaskReader{client: h.client} }

func (h *grpcHostClient) Sessions() SessionReader { return grpcSessionReader{client: h.client} }

func (h *grpcHostClient) Workspaces() WorkspaceReader { return grpcWorkspaceReader{client: h.client} }

func (h *grpcHostClient) Workflows() WorkflowReader { return grpcWorkflowReader{client: h.client} }

func (h *grpcHostClient) AgentProfiles() AgentProfileReader {
	return grpcAgentProfileReader{client: h.client}
}

func (h *grpcHostClient) Repositories() RepositoryReader {
	return grpcRepositoryReader{client: h.client}
}

var _ Host = (*grpcHostClient)(nil)

// grpcTaskReader implements TaskReader on the plugin side, calling the
// generated pluginv1.HostClient and converting proto<->Go-native.
type grpcTaskReader struct {
	client pluginv1.HostClient
}

func (r grpcTaskReader) List(ctx context.Context, filter TaskFilter, page Page) ([]Task, *PageInfo, error) {
	resp, err := r.client.ListTasks(ctx, &pluginv1.ListTasksRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	tasks, err := tasksFromProto(resp.GetTasks())
	if err != nil {
		return nil, nil, err
	}
	return tasks, pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcTaskReader) Get(ctx context.Context, id string) (*Task, error) {
	resp, err := r.client.GetTask(ctx, &pluginv1.GetTaskRequest{Id: id})
	if err != nil {
		return nil, err
	}
	task, err := taskFromProto(resp.GetTask())
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// grpcSessionReader implements SessionReader on the plugin side.
type grpcSessionReader struct {
	client pluginv1.HostClient
}

func (r grpcSessionReader) List(ctx context.Context, filter SessionFilter, page Page) ([]Session, *PageInfo, error) {
	resp, err := r.client.ListSessions(ctx, &pluginv1.ListSessionsRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return sessionsFromProto(resp.GetSessions()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcSessionReader) CodeStats(ctx context.Context, filter SessionFilter, page Page) ([]SessionCodeStats, *PageInfo, error) {
	resp, err := r.client.ListSessionCodeStats(ctx, &pluginv1.ListSessionCodeStatsRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return sessionCodeStatsSliceFromProto(resp.GetStats()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcWorkspaceReader implements WorkspaceReader on the plugin side.
type grpcWorkspaceReader struct {
	client pluginv1.HostClient
}

func (r grpcWorkspaceReader) List(ctx context.Context, page Page) ([]Workspace, *PageInfo, error) {
	resp, err := r.client.ListWorkspaces(ctx, &pluginv1.ListWorkspacesRequest{Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return workspacesFromProto(resp.GetWorkspaces()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcWorkflowReader implements WorkflowReader on the plugin side.
type grpcWorkflowReader struct {
	client pluginv1.HostClient
}

func (r grpcWorkflowReader) List(ctx context.Context, workspaceID string, page Page) ([]Workflow, *PageInfo, error) {
	resp, err := r.client.ListWorkflows(ctx, &pluginv1.ListWorkflowsRequest{WorkspaceId: workspaceID, Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return workflowsFromProto(resp.GetWorkflows()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcWorkflowReader) ListSteps(ctx context.Context, workflowID string) ([]WorkflowStep, error) {
	resp, err := r.client.ListWorkflowSteps(ctx, &pluginv1.ListWorkflowStepsRequest{WorkflowId: workflowID})
	if err != nil {
		return nil, err
	}
	return workflowStepsFromProto(resp.GetSteps()), nil
}

// grpcAgentProfileReader implements AgentProfileReader on the plugin side.
type grpcAgentProfileReader struct {
	client pluginv1.HostClient
}

func (r grpcAgentProfileReader) List(ctx context.Context, page Page) ([]AgentProfile, *PageInfo, error) {
	resp, err := r.client.ListAgentProfiles(ctx, &pluginv1.ListAgentProfilesRequest{Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return agentProfilesFromProto(resp.GetProfiles()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcRepositoryReader implements RepositoryReader on the plugin side.
type grpcRepositoryReader struct {
	client pluginv1.HostClient
}

func (r grpcRepositoryReader) List(ctx context.Context, workspaceID string, page Page) ([]Repository, *PageInfo, error) {
	resp, err := r.client.ListRepositories(ctx, &pluginv1.ListRepositoriesRequest{WorkspaceId: workspaceID, Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return repositoriesFromProto(resp.GetRepositories()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// registerHostServer registers a grpc server that dispatches
// kandev.plugin.v1.Host RPCs to impl (kandev's Go-native Host
// implementation), converting proto<->Go-native types at the boundary.
func registerHostServer(s grpc.ServiceRegistrar, impl Host) {
	pluginv1.RegisterHostServer(s, &grpcHostServer{impl: impl})
}

type grpcHostServer struct {
	pluginv1.UnimplementedHostServer
	impl Host
}

func (s *grpcHostServer) GetState(ctx context.Context, req *pluginv1.GetStateRequest) (*pluginv1.GetStateResponse, error) {
	value, found, err := s.impl.GetState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey())
	if err != nil {
		return nil, err
	}
	if !found {
		return &pluginv1.GetStateResponse{Found: false}, nil
	}
	protoValue, err := mapToStruct(value)
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetStateResponse{Found: true, Value: protoValue}, nil
}

func (s *grpcHostServer) SetState(ctx context.Context, req *pluginv1.SetStateRequest) (*pluginv1.SetStateResponse, error) {
	value, err := structToMap(req.GetValue())
	if err != nil {
		return nil, err
	}
	if err := s.impl.SetState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey(), value); err != nil {
		return nil, err
	}
	return &pluginv1.SetStateResponse{}, nil
}

func (s *grpcHostServer) DeleteState(ctx context.Context, req *pluginv1.DeleteStateRequest) (*pluginv1.DeleteStateResponse, error) {
	if err := s.impl.DeleteState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey()); err != nil {
		return nil, err
	}
	return &pluginv1.DeleteStateResponse{}, nil
}

func (s *grpcHostServer) ListState(ctx context.Context, req *pluginv1.ListStateRequest) (*pluginv1.ListStateResponse, error) {
	entries, err := s.impl.ListState(ctx, req.GetScope(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	protoEntries := make([]*pluginv1.StateEntry, len(entries))
	for i := range entries {
		converted, err := entries[i].toProto()
		if err != nil {
			return nil, err
		}
		protoEntries[i] = converted
	}
	return &pluginv1.ListStateResponse{Entries: protoEntries}, nil
}

func (s *grpcHostServer) RevealSecret(ctx context.Context, req *pluginv1.RevealSecretRequest) (*pluginv1.RevealSecretResponse, error) {
	value, err := s.impl.RevealSecret(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	return &pluginv1.RevealSecretResponse{Value: value}, nil
}

func (s *grpcHostServer) EmitEvent(ctx context.Context, req *pluginv1.EmitEventRequest) (*pluginv1.EmitEventResponse, error) {
	payload, err := structToMap(req.GetPayload())
	if err != nil {
		return nil, err
	}
	if err := s.impl.EmitEvent(ctx, req.GetEventName(), payload); err != nil {
		return nil, err
	}
	return &pluginv1.EmitEventResponse{}, nil
}

// ── Host data API reads (ADR 0043) ──────────────────────────────────────
//
// Each method below dispatches to the injected Go-native impl's resource
// accessor (impl.Tasks(), impl.Sessions(), ...) and converts proto<->native
// at the boundary, exactly like GetState/ListState above. Capability
// gating and the real service-layer calls live in the impl kandev's
// runtime manager provides (internal/plugins), not here — this adapter
// only wires the RPC to whatever impl.Tasks()/impl.Sessions()/... does, so
// it compiles against any Host, including one that embeds
// UnimplementedHostData and returns Unimplemented for every accessor.

func (s *grpcHostServer) ListTasks(ctx context.Context, req *pluginv1.ListTasksRequest) (*pluginv1.ListTasksResponse, error) {
	filter := taskFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	tasks, pageInfo, err := s.impl.Tasks().List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	protoTasks, err := tasksToProto(tasks)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListTasksResponse{Tasks: protoTasks, PageInfo: pageInfo.toProto()}, nil
}

// GetTask dispatches to the impl's TaskReader.Get and wraps the result in
// GetTaskResponse (Buf RPC_RESPONSE_STANDARD_NAME/RPC_REQUEST_RESPONSE_UNIQUE:
// no bare-DTO RPC responses). The Host contract is that a missing task is a
// gRPC NotFound *error* from Get, never a (nil, nil) success — the nil check
// below is defense-in-depth for a Host implementation that doesn't follow
// that contract, so a plugin still gets NotFound instead of a zero-value
// GetTaskResponse.
func (s *grpcHostServer) GetTask(ctx context.Context, req *pluginv1.GetTaskRequest) (*pluginv1.GetTaskResponse, error) {
	task, err := s.impl.Tasks().Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	protoTask, err := task.toProto()
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetTaskResponse{Task: protoTask}, nil
}

func (s *grpcHostServer) ListWorkspaces(ctx context.Context, req *pluginv1.ListWorkspacesRequest) (*pluginv1.ListWorkspacesResponse, error) {
	page := pageFromProto(req.GetPage())
	workspaces, pageInfo, err := s.impl.Workspaces().List(ctx, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListWorkspacesResponse{Workspaces: workspacesToProto(workspaces), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListWorkflows(ctx context.Context, req *pluginv1.ListWorkflowsRequest) (*pluginv1.ListWorkflowsResponse, error) {
	page := pageFromProto(req.GetPage())
	workflows, pageInfo, err := s.impl.Workflows().List(ctx, req.GetWorkspaceId(), page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListWorkflowsResponse{Workflows: workflowsToProto(workflows), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListWorkflowSteps(ctx context.Context, req *pluginv1.ListWorkflowStepsRequest) (*pluginv1.ListWorkflowStepsResponse, error) {
	steps, err := s.impl.Workflows().ListSteps(ctx, req.GetWorkflowId())
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListWorkflowStepsResponse{Steps: workflowStepsToProto(steps)}, nil
}

func (s *grpcHostServer) ListAgentProfiles(ctx context.Context, req *pluginv1.ListAgentProfilesRequest) (*pluginv1.ListAgentProfilesResponse, error) {
	page := pageFromProto(req.GetPage())
	profiles, pageInfo, err := s.impl.AgentProfiles().List(ctx, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListAgentProfilesResponse{Profiles: agentProfilesToProto(profiles), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListRepositories(ctx context.Context, req *pluginv1.ListRepositoriesRequest) (*pluginv1.ListRepositoriesResponse, error) {
	page := pageFromProto(req.GetPage())
	repos, pageInfo, err := s.impl.Repositories().List(ctx, req.GetWorkspaceId(), page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListRepositoriesResponse{Repositories: repositoriesToProto(repos), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListSessions(ctx context.Context, req *pluginv1.ListSessionsRequest) (*pluginv1.ListSessionsResponse, error) {
	filter := sessionFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	sessions, pageInfo, err := s.impl.Sessions().List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListSessionsResponse{Sessions: sessionsToProto(sessions), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListSessionCodeStats(ctx context.Context, req *pluginv1.ListSessionCodeStatsRequest) (*pluginv1.ListSessionCodeStatsResponse, error) {
	filter := sessionFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	stats, pageInfo, err := s.impl.Sessions().CodeStats(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListSessionCodeStatsResponse{Stats: sessionCodeStatsSliceToProto(stats), PageInfo: pageInfo.toProto()}, nil
}

var _ pluginv1.HostServer = (*grpcHostServer)(nil)

// UnimplementedHostData is an embeddable default for the Host data API
// (ADR 0043) sub-accessors: Tasks/Sessions/Workspaces/Workflows/
// AgentProfiles/Repositories. Embed it in a Go-native Host implementation
// to satisfy the interface before wiring real data access — every method
// on every returned reader returns a gRPC Unimplemented error. Override
// individual accessor methods (e.g. define your own Tasks() on the
// embedding type) as real capability-gated, service-backed logic lands;
// unlike UnimplementedPlugin's per-RPC methods, these are per-resource
// accessors because each one fans out to multiple reader methods.
type UnimplementedHostData struct{}

func (UnimplementedHostData) Tasks() TaskReader           { return unimplementedTaskReader{} }
func (UnimplementedHostData) Sessions() SessionReader     { return unimplementedSessionReader{} }
func (UnimplementedHostData) Workspaces() WorkspaceReader { return unimplementedWorkspaceReader{} }
func (UnimplementedHostData) Workflows() WorkflowReader   { return unimplementedWorkflowReader{} }
func (UnimplementedHostData) AgentProfiles() AgentProfileReader {
	return unimplementedAgentProfileReader{}
}
func (UnimplementedHostData) Repositories() RepositoryReader {
	return unimplementedRepositoryReader{}
}

func errUnimplementedHostData(resource string) error {
	return status.Errorf(codes.Unimplemented, "pluginsdk: Host data API %q not implemented", resource)
}

type unimplementedTaskReader struct{}

func (unimplementedTaskReader) List(context.Context, TaskFilter, Page) ([]Task, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("tasks")
}

func (unimplementedTaskReader) Get(context.Context, string) (*Task, error) {
	return nil, errUnimplementedHostData("tasks")
}

type unimplementedSessionReader struct{}

func (unimplementedSessionReader) List(context.Context, SessionFilter, Page) ([]Session, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("sessions")
}

func (unimplementedSessionReader) CodeStats(context.Context, SessionFilter, Page) ([]SessionCodeStats, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("sessions")
}

type unimplementedWorkspaceReader struct{}

func (unimplementedWorkspaceReader) List(context.Context, Page) ([]Workspace, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("workspaces")
}

type unimplementedWorkflowReader struct{}

func (unimplementedWorkflowReader) List(context.Context, string, Page) ([]Workflow, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("workflows")
}

func (unimplementedWorkflowReader) ListSteps(context.Context, string) ([]WorkflowStep, error) {
	return nil, errUnimplementedHostData("workflows")
}

type unimplementedAgentProfileReader struct{}

func (unimplementedAgentProfileReader) List(context.Context, Page) ([]AgentProfile, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("agent_profiles")
}

type unimplementedRepositoryReader struct{}

func (unimplementedRepositoryReader) List(context.Context, string, Page) ([]Repository, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("repositories")
}
