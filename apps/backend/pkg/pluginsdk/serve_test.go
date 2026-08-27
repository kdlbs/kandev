package pluginsdk

import (
	"context"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAuthorPlugin is a minimal author-facing Plugin used to exercise the
// real go-plugin gRPC + broker wiring end to end (no subprocess spawn —
// hcplugin.TestPluginGRPCConn wires a real unix-socket grpc.Server/Client
// pair with a real GRPCBroker, matching production transport behavior).
type fakeAuthorPlugin struct {
	UnimplementedPlugin
	events  []*Event
	actions []*PluginActionRequest
}

func (p *fakeAuthorPlugin) OnEvent(_ context.Context, e *Event) error {
	p.events = append(p.events, e)
	return nil
}

func (p *fakeAuthorPlugin) HandleWebhook(_ context.Context, req *WebhookRequest) (*WebhookResponse, error) {
	return &WebhookResponse{Status: 200, Body: append([]byte("got:"), req.Body...)}, nil
}

func (p *fakeAuthorPlugin) HandleAction(_ context.Context, req *PluginActionRequest) (*PluginActionResponse, error) {
	p.actions = append(p.actions, req)
	return &PluginActionResponse{Body: append([]byte(req.Context.ActorID+":"), req.Body...), Headers: map[string]string{"Content-Type": "application/json"}}, nil
}

func (*fakeAuthorPlugin) SearchEntityReferences(_ context.Context, req *SearchEntityReferencesRequest) (*SearchEntityReferencesResponse, error) {
	return &SearchEntityReferencesResponse{Candidates: []EntityReferenceCandidate{{ProviderLocalID: req.Source + "-1", Title: "PR 1", Attributes: map[string]any{"query": req.Query}}}}, nil
}

func (*fakeAuthorPlugin) AuthorizeEntityReference(_ context.Context, req *AuthorizeEntityReferenceRequest) (*AuthorizeEntityReferenceResponse, error) {
	return &AuthorizeEntityReferenceResponse{Allowed: req.Purpose == "submission"}, nil
}

func (*fakeAuthorPlugin) ResolveGitCredential(_ context.Context, req *ResolveGitCredentialRequest) (*ResolveGitCredentialResponse, error) {
	return &ResolveGitCredentialResponse{Username: req.ProviderID, Secret: "transient", ExpiresAt: "2026-07-31T12:00:00Z"}, nil
}

func (*fakeAuthorPlugin) GetGitCredentialBinding(_ context.Context, _ *GitCredentialBindingRequest) (*GitCredentialBindingResponse, error) {
	return &GitCredentialBindingResponse{Binding: "connection:7"}, nil
}

func (p *fakeAuthorPlugin) InvokeAgentTool(_ context.Context, req *AgentToolRequest) (*AgentToolResult, error) {
	return &AgentToolResult{
		Text: "called:" + req.Context.TaskID,
		StructuredContent: map[string]any{
			"name": req.Name, "value": req.Arguments["value"],
		},
	}, nil
}

type nilAgentToolPlugin struct{ UnimplementedPlugin }

func (*nilAgentToolPlugin) InvokeAgentTool(context.Context, *AgentToolRequest) (*AgentToolResult, error) {
	return nil, nil
}

func TestGRPCPluginServerRejectsNilAgentToolResult(t *testing.T) {
	server := &grpcPluginServer{impl: &nilAgentToolPlugin{}}
	_, err := server.InvokeAgentTool(context.Background(), &pluginv1.AgentToolRequest{Name: "echo"})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

// TestServe_EndToEnd exercises the same GRPCPlugin wiring Serve() (plugin
// side) and kandev's runtime manager (host side, via plugin.NewClient)
// use, over a real go-plugin gRPC connection + broker
// (hcplugin.TestPluginGRPCConn). It covers both Plugin RPCs plus the
// Host broker round trip (§3/§4 of docs/plans/plugins/GRPC-CONTRACT.md).
func TestServe_EndToEnd(t *testing.T) {
	author := &fakeAuthorPlugin{}
	hostImpl := &recordingHost{
		getStateFn: func(_ context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
			if scope == "task" && scopeID == "t1" && key == "k1" {
				return map[string]any{"greeting": "hi"}, true, nil
			}
			return nil, false, nil
		},
	}
	// hcplugin.TestPluginGRPCConn dispenses/serves the SAME Plugin value on
	// both ends of the harness (there's no real process boundary), so one
	// GRPCPlugin with both Impl and Host set plays both roles: GRPCServer
	// runs as the "plugin subprocess" would, GRPCClient runs as "kandev"
	// would. In production these are two separate processes each setting
	// only one of Impl/Host.
	gp := &GRPCPlugin{Impl: author, Host: hostImpl, HostDialTimeout: 5 * time.Second}

	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		PluginMapKey: gp,
	})
	defer func() { _ = client.Close() }()
	defer server.Stop()

	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote, ok := raw.(*RemotePlugin)
	require.True(t, ok, "Dispense(%q) should return *RemotePlugin, got %T", PluginMapKey, raw)

	t.Run("DeliverEvent", func(t *testing.T) {
		err := remote.DeliverEvent(context.Background(), &Event{
			EventID:   "e1",
			EventType: "task.created",
			Payload:   map[string]any{"a": float64(1)},
		})
		require.NoError(t, err)
		require.Len(t, author.events, 1)
		require.Equal(t, "e1", author.events[0].EventID)
		require.Equal(t, map[string]any{"a": float64(1)}, author.events[0].Payload)
	})

	t.Run("HandleWebhook", func(t *testing.T) {
		resp, err := remote.HandleWebhook(context.Background(), &WebhookRequest{Body: []byte("hi")})
		require.NoError(t, err)
		require.Equal(t, int32(200), resp.Status)
		require.Equal(t, []byte("got:hi"), resp.Body)
	})

	t.Run("OptionalPluginExtensions", func(t *testing.T) {
		action, err := remote.HandleAction(context.Background(), &PluginActionRequest{
			ActionKey: "connection.get",
			Context: VerifiedActionContext{
				ActorID: "user-1", WorkspaceID: "ws-1", TaskID: "task-1",
				RepositoryID: "repository-1", SessionID: "session-1", HeadBranch: "feature/native-create",
			},
			Body: []byte(`{"connected":true}`),
		})
		require.NoError(t, err)
		require.Equal(t, []byte(`user-1:{"connected":true}`), action.Body)
		require.Equal(t, "application/json", action.Headers["Content-Type"])
		require.Equal(t, "session-1", author.actions[0].Context.SessionID)
		require.Equal(t, "feature/native-create", author.actions[0].Context.HeadBranch)

		search, err := remote.SearchEntityReferences(context.Background(), &SearchEntityReferencesRequest{Source: "bitbucket", WorkspaceID: "ws-1", Query: "build", Limit: 5})
		require.NoError(t, err)
		require.Equal(t, []EntityReferenceCandidate{{ProviderLocalID: "bitbucket-1", Title: "PR 1", Attributes: map[string]any{"query": "build"}}}, search.Candidates)

		authorized, err := remote.AuthorizeEntityReference(context.Background(), &AuthorizeEntityReferenceRequest{Source: "bitbucket", WorkspaceID: "ws-1", Purpose: "submission", Reference: map[string]any{"id": "1"}})
		require.NoError(t, err)
		require.True(t, authorized.Allowed)

		credential, err := remote.ResolveGitCredential(context.Background(), &ResolveGitCredentialRequest{ProviderID: "bitbucket", WorkspaceID: "ws-1", Host: "bitbucket.org", Path: "/team/repo.git"})
		require.NoError(t, err)
		require.Equal(t, &ResolveGitCredentialResponse{Username: "bitbucket", Secret: "transient", ExpiresAt: "2026-07-31T12:00:00Z"}, credential)

		binding, err := remote.GetGitCredentialBinding(context.Background(), &GitCredentialBindingRequest{ProviderID: "bitbucket", WorkspaceID: "ws-1", Host: "bitbucket.org", Path: "/team/repo.git"})
		require.NoError(t, err)
		require.Equal(t, "connection:7", binding.Binding)
	})

	t.Run("InvokeAgentTool", func(t *testing.T) {
		resp, err := remote.InvokeAgentTool(context.Background(), &AgentToolRequest{
			InvocationID: "inv-1", Name: "echo", Arguments: map[string]any{"value": "ok"},
			Context: AgentToolContext{TaskID: "task-1", SessionID: "session-1", WorkspaceID: "workspace-1", Surface: "kanban-task"},
		})
		require.NoError(t, err)
		require.Equal(t, "called:task-1", resp.Text)
		require.Equal(t, "echo", resp.StructuredContent["name"])
	})

	t.Run("HostBrokerRoundTrip", func(t *testing.T) {
		require.Eventually(t, func() bool {
			return author.Host() != nil
		}, 5*time.Second, 10*time.Millisecond, "Serve-equivalent GRPCServer should inject Host via the broker")

		value, found, err := author.Host().GetState(context.Background(), "task", "t1", "k1")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, map[string]any{"greeting": "hi"}, value)

		_, found, err = author.Host().GetState(context.Background(), "task", "t1", "missing")
		require.NoError(t, err)
		require.False(t, found)
	})
}

// TestServe_LegacyPluginGetsClearUnsupportedExtensionErrors confirms that
// adding optional RPCs does not force existing Plugin implementations to add
// methods. Calls to an extension they did not opt into fail explicitly rather
// than looking like transport failure or a successful empty response.
func TestServe_LegacyPluginGetsClearUnsupportedExtensionErrors(t *testing.T) {
	gp := &GRPCPlugin{Impl: &UnimplementedPlugin{}}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		PluginMapKey: gp,
	})
	defer func() { _ = client.Close() }()
	defer server.Stop()

	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote, ok := raw.(*RemotePlugin)
	require.True(t, ok)

	_, err = remote.HandleAction(context.Background(), &PluginActionRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "plugin does not implement action handler")

	_, err = remote.SearchEntityReferences(context.Background(), &SearchEntityReferencesRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "plugin does not implement entity reference searcher")

	_, err = remote.AuthorizeEntityReference(context.Background(), &AuthorizeEntityReferenceRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "plugin does not implement entity reference authorizer")

	_, err = remote.ResolveGitCredential(context.Background(), &ResolveGitCredentialRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "plugin does not implement git credential resolver")

	_, err = remote.GetGitCredentialBinding(context.Background(), &GitCredentialBindingRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "plugin does not implement git credential binder")
}
