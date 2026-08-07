package plugins

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/mentions"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type fakeMentionSourceClient struct {
	records   []*store.Record
	search    func(context.Context, string, *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error)
	authorize func(context.Context, string, *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error)
}

func (c *fakeMentionSourceClient) List() []*store.Record {
	return c.records
}

func (c *fakeMentionSourceClient) SearchEntityReferences(
	ctx context.Context, id string, _ pluginDispatchGeneration, request *pluginsdk.SearchEntityReferencesRequest,
) (*pluginsdk.SearchEntityReferencesResponse, error) {
	return c.search(ctx, id, request)
}

func (c *fakeMentionSourceClient) AuthorizeEntityReference(
	ctx context.Context, id string, _ pluginDispatchGeneration, request *pluginsdk.AuthorizeEntityReferenceRequest,
) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
	return c.authorize(ctx, id, request)
}

func TestMentionSourceBridgeCanonicalizesPluginCandidatesWithVerifiedSearchScope(t *testing.T) {
	client := &fakeMentionSourceClient{
		records: []*store.Record{testMentionSourceRecord()},
		search: func(_ context.Context, id string, request *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error) {
			if id != "kandev-plugin-bitbucket" || request.Source != "bitbucket" ||
				request.WorkspaceID != "workspace-1" || request.Query != "auth" || request.Limit != 3 {
				t.Errorf("search request = %#v for %q", request, id)
				return &pluginsdk.SearchEntityReferencesResponse{}, nil
			}
			return &pluginsdk.SearchEntityReferencesResponse{Candidates: []pluginsdk.EntityReferenceCandidate{{
				ProviderLocalID: "pull-42",
				Title:           "Fix authentication",
				URL:             "https://bitbucket.example/projects/APP/repos/web/pull-requests/42",
				Attributes:      map[string]any{"provider": "attacker", "scope": "workspace-2"},
			}}}, nil
		},
		authorize: func(_ context.Context, id string, request *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
			if id != "kandev-plugin-bitbucket" || request.Source != "bitbucket" ||
				request.WorkspaceID != "workspace-1" || request.Purpose != "search" {
				t.Errorf("authorization request = %#v for %q", request, id)
				return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false}, nil
			}
			return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: true}, nil
		},
	}
	registry := mentions.NewRegistry()
	bridge := NewMentionSourceBridge(client)
	if err := bridge.RegisterMentionSources(registry); err != nil {
		t.Fatalf("register mention sources: %v", err)
	}

	response, err := mentions.NewService(registry).Search(context.Background(), mentions.SearchRequest{
		WorkspaceID: "workspace-1", Query: "auth", Limit: 3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 {
		t.Fatalf("response = %#v, want one result", response)
	}
	result := response.Groups[0].Results[0]
	if result.Provider != "bitbucket" || result.Kind != "pull_request" || result.Scope != "workspace-1" ||
		result.ID != "pull-42" || result.Ref != "mention:v1:bitbucket:pull_request:workspace-1:pull-42" {
		t.Fatalf("host canonical reference = %#v", result)
	}
}

func TestMentionSourceBridgeReauthorizesSubmissionAndRejectsStaleOrTamperedReferences(t *testing.T) {
	authorizeCalls := 0
	client := &fakeMentionSourceClient{
		records: []*store.Record{testMentionSourceRecord()},
		search: func(context.Context, string, *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error) {
			return &pluginsdk.SearchEntityReferencesResponse{}, nil
		},
		authorize: func(ctx context.Context, _ string, request *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
			authorizeCalls++
			if request.Purpose != "submission" {
				t.Errorf("purpose = %q, want submission", request.Purpose)
				return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false}, nil
			}
			if request.WorkspaceID != "workspace-1" || request.Source != "bitbucket" {
				t.Errorf("submission request = %#v", request)
				return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false}, nil
			}
			if request.Reference["id"] == "timeout" {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: request.Reference["id"] == "pull-42"}, nil
		},
	}
	registry := mentions.NewRegistry()
	bridge := NewMentionSourceBridge(client)
	if err := bridge.RegisterMentionSources(registry); err != nil {
		t.Fatalf("register mention sources: %v", err)
	}
	valid := pluginReference("pull-42", "workspace-1")
	if err := registry.AuthorizeForWorkspace(context.Background(), "workspace-1", valid); err != nil {
		t.Fatalf("authorize valid reference: %v", err)
	}

	tampered := pluginReference("pull-43", "workspace-1")
	if err := registry.AuthorizeForWorkspace(context.Background(), "workspace-1", tampered); !errors.Is(err, mentions.ErrReferenceUnauthorized) {
		t.Fatalf("tampered reference error = %v, want unauthorized", err)
	}
	crossWorkspace := pluginReference("pull-42", "workspace-2")
	if err := registry.AuthorizeForWorkspace(context.Background(), "workspace-1", crossWorkspace); !errors.Is(err, mentions.ErrReferenceUnauthorized) {
		t.Fatalf("cross-workspace reference error = %v, want unauthorized", err)
	}
	if authorizeCalls != 2 {
		t.Fatalf("plugin authorization calls = %d, want no cross-workspace call", authorizeCalls)
	}
	spoofed := valid
	spoofed.Provider = "attacker"
	if err := registry.AuthorizeForWorkspace(context.Background(), "workspace-1", spoofed); !errors.Is(err, mentions.ErrReferenceProviderUnavailable) {
		t.Fatalf("spoofed provider error = %v, want unavailable", err)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := registry.AuthorizeForWorkspace(timeoutCtx, "workspace-1", pluginReference("timeout", "workspace-1")); !errors.Is(err, mentions.ErrReferenceUnauthorized) {
		t.Fatalf("timed-out reference error = %v, want unauthorized", err)
	}
	client.records = nil
	if err := bridge.Sync(); err != nil {
		t.Fatalf("remove disabled plugin sources: %v", err)
	}
	if err := registry.AuthorizeForWorkspace(context.Background(), "workspace-1", valid); !errors.Is(err, mentions.ErrReferenceProviderUnavailable) {
		t.Fatalf("disabled plugin error = %v, want unavailable", err)
	}
}

func TestMentionSourceBridgeTransfersOwnershipInOneSync(t *testing.T) {
	predecessor := testMentionSourceRecord()
	predecessor.ID = "z-predecessor"
	successor := testMentionSourceRecord()
	successor.ID = "a-successor"
	client := &fakeMentionSourceClient{
		records: []*store.Record{predecessor},
		search: func(context.Context, string, *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error) {
			return &pluginsdk.SearchEntityReferencesResponse{}, nil
		},
		authorize: func(context.Context, string, *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
			return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: true}, nil
		},
	}
	bridge := NewMentionSourceBridge(client)
	if err := bridge.RegisterMentionSources(mentions.NewRegistry()); err != nil {
		t.Fatalf("register predecessor: %v", err)
	}

	client.records = []*store.Record{successor}
	if err := bridge.Sync(); err != nil {
		t.Fatalf("transfer source ownership: %v", err)
	}
	if _, retained := bridge.owners[predecessor.ID]; retained {
		t.Fatalf("departed owner %q was retained", predecessor.ID)
	}
	if _, active := bridge.owners[successor.ID]; !active {
		t.Fatalf("successor owner %q was not installed by the first sync", successor.ID)
	}
}

func pluginReference(id, workspaceID string) apiv1.EntityReference {
	return apiv1.EntityReference{
		Provider: "bitbucket", Kind: "pull_request", ID: id, Scope: workspaceID,
		Title: "Fix authentication", URL: "https://bitbucket.example/pull-requests/42",
	}
}

func testMentionSourceRecord() *store.Record {
	return &store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-plugin-bitbucket",
			ReferenceSources: []manifest.ReferenceSource{{
				Source: "bitbucket", Provider: "bitbucket", Kind: "pull_request",
				DisplayName: "Bitbucket", KindLabel: "Pull request", Order: 80,
			}},
		},
		Status: StatusActive,
	}
}
