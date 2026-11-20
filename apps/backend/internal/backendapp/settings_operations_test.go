package backendapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/settingscatalog"
	storage "github.com/kandev/kandev/internal/system/storage"
)

func TestSettingsMutationRequiresCallerIdentity(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	operations := &settingsOperations{registry: registry}
	target := settingscatalog.ResourceTarget{ResourceType: "user_settings"}
	changes := map[string]json.RawMessage{"confirm_task_archive": json.RawMessage(`false`)}
	if err := operations.authorizeSettingsMutation(context.Background(), target, changes); err == nil {
		t.Fatal("mutation without caller identity was accepted")
	}
	if err := operations.authorizeSettingsMutation(
		authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true}), target, changes,
	); err != nil {
		t.Fatalf("synthetic identity was rejected: %v", err)
	}
}

func TestGitHubSettingsOperationsScopeUnion(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service := &recordingScopeUnionGitHubService{current: &github.WorkspaceSettings{
		WorkspaceID: "ws-1", RepoScopeMode: github.RepoScopeModeAll,
	}}
	operations := &settingsOperations{registry: registry, deps: settingsDomainDependencies{github: service}}
	workspaceID := "ws-1"
	updated, err := operations.updateIntegrationSettings(context.Background(), settingscatalog.ResourceTarget{
		ResourceType: "github_settings", WorkspaceID: &workspaceID,
	}, map[string]json.RawMessage{
		"repo_scope_mode":  json.RawMessage(`"selected"`),
		"repo_scope_orgs":  json.RawMessage(`["Acme"]`),
		"repo_scope_repos": json.RawMessage(`[{"owner":"outside","name":"tool"}]`),
	}, nil)
	if err != nil {
		t.Fatalf("update GitHub settings through settings operations: %v", err)
	}
	response, ok := updated.(map[string]any)
	if !ok || response["settings"] == nil {
		t.Fatalf("settings operations response = %#v, want projected settings", updated)
	}
	if service.request == nil || service.request.RepoScopeMode == nil || *service.request.RepoScopeMode != github.RepoScopeModeSelected || service.request.RepoScopeOrgs == nil || len(*service.request.RepoScopeOrgs) != 1 || service.request.RepoScopeRepos == nil || len(*service.request.RepoScopeRepos) != 1 {
		t.Fatalf("forwarded scope request = %+v, want a mixed selected patch", service.request)
	}
}

type recordingScopeUnionGitHubService struct {
	settingsGitHubService
	current *github.WorkspaceSettings
	request *github.UpdateWorkspaceSettingsRequest
	updated *github.WorkspaceSettings
}

func (s *recordingScopeUnionGitHubService) GetWorkspaceSettings(context.Context, string) (*github.WorkspaceSettings, error) {
	return s.current, nil
}

func (s *recordingScopeUnionGitHubService) UpdateWorkspaceSettings(_ context.Context, request *github.UpdateWorkspaceSettingsRequest) (*github.WorkspaceSettings, error) {
	s.request = request
	s.updated = &github.WorkspaceSettings{WorkspaceID: request.WorkspaceID, RepoScopeMode: github.RepoScopeModeSelected}
	if request.RepoScopeOrgs != nil {
		s.updated.RepoScopeOrgs = append([]string(nil), (*request.RepoScopeOrgs)...)
	}
	if request.RepoScopeRepos != nil {
		s.updated.RepoScopeRepos = append([]github.RepoFilter(nil), (*request.RepoScopeRepos)...)
	}
	return s.updated, nil
}

func TestSettingsSensitiveValuesAreRedactedWithoutDroppingReferences(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"env_vars":       []any{map[string]any{"key": "TOKEN", "value": "secret-value", "secret_id": "secret-1"}},
		"config_options": map[string]any{"safe": "kept"},
	}
	redactSettingsValues(values, registry, "agent_profile")

	encoded, _ := json.Marshal(values)
	if strings.Contains(string(encoded), "secret-value") {
		t.Fatalf("sensitive value leaked: %s", encoded)
	}
	entry := values["env_vars"].([]any)[0].(map[string]any)
	if entry["secret_id"] != "secret-1" || entry["redacted"] != true {
		t.Fatalf("secret reference was not preserved safely: %#v", entry)
	}
}

func TestRuntimeFlagSettingsValueUsesCatalogOverridePath(t *testing.T) {
	override := true
	value := runtimeFlagSettingsValue(runtimeflags.RuntimeFlagState{
		Key:           "features.example",
		OverrideValue: &override,
	})
	if value["override"] != true {
		t.Fatalf("override = %#v, want true", value["override"])
	}
	if _, ok := value["override_value"]; ok {
		t.Fatalf("service-specific override_value leaked into settings contract: %#v", value)
	}
}

type recordingSettingsRuntimeFlags struct {
	setOverrideCalls int
}

func (r *recordingSettingsRuntimeFlags) ListStates(context.Context) ([]runtimeflags.RuntimeFlagState, error) {
	return []runtimeflags.RuntimeFlagState{{Key: "features.auth"}}, nil
}

func (r *recordingSettingsRuntimeFlags) SetOverride(context.Context, string, *bool) ([]runtimeflags.RuntimeFlagState, error) {
	r.setOverrideCalls++
	return r.ListStates(context.Background())
}

func TestRuntimeFlagSettingsCannotMutateAuthenticationEnablement(t *testing.T) {
	for _, raw := range []string{`true`, `false`, `null`} {
		t.Run(raw, func(t *testing.T) {
			flags := &recordingSettingsRuntimeFlags{}
			operations := &settingsOperations{deps: settingsDomainDependencies{runtimeFlags: flags}}

			_, err := operations.updateRuntimeFlag(context.Background(), "features.auth", map[string]json.RawMessage{
				"override": json.RawMessage(raw),
			})
			if err == nil {
				t.Fatal("authentication runtime flag mutation was accepted")
			}
			if flags.setOverrideCalls != 0 {
				t.Fatalf("SetOverride calls = %d, want 0", flags.setOverrideCalls)
			}
		})
	}
}

type recordingStorageSettings struct {
	getCalls   int
	saveCalls  int
	patchCalls int
	patched    map[string]json.RawMessage
}

func (s *recordingStorageSettings) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	s.getCalls++
	return storage.DefaultSettings(), nil
}

func (s *recordingStorageSettings) SaveSettingsWithConfirmations(context.Context, storage.StorageMaintenanceSettings, storage.SaveConfirmations) (storage.StorageMaintenanceSettings, error) {
	s.saveCalls++
	return storage.DefaultSettings(), nil
}

func (s *recordingStorageSettings) PatchSettingsWithConfirmations(_ context.Context, changes map[string]json.RawMessage, _ storage.SaveConfirmations) (storage.StorageMaintenanceSettings, error) {
	s.patchCalls++
	s.patched = changes
	updated := storage.DefaultSettings()
	updated.Enabled = true
	return updated, nil
}

func TestStorageSettingsUpdateUsesAtomicStoragePatch(t *testing.T) {
	store := &recordingStorageSettings{}
	operations := &settingsOperations{deps: settingsDomainDependencies{storage: store}}

	updated, err := operations.updateStorageSettings(context.Background(), map[string]json.RawMessage{
		"enabled":                    json.RawMessage(`true`),
		"docker.build_cache_enabled": json.RawMessage(`false`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.patchCalls != 1 || store.getCalls != 0 || store.saveCalls != 0 {
		t.Fatalf("storage calls = patch:%d get:%d save:%d, want one atomic patch only", store.patchCalls, store.getCalls, store.saveCalls)
	}
	if len(store.patched) != 2 || updated == nil {
		t.Fatalf("patched changes = %#v, updated = %#v", store.patched, updated)
	}
}

func TestDecodeUserSettingsUpdateMapsSidebarTaskColors(t *testing.T) {
	request, err := decodeUserSettingsUpdate(map[string]json.RawMessage{
		"user_settings.sidebar_task_colors": json.RawMessage(`{"task-1":"blue","task-2":null}`),
	})
	if err != nil {
		t.Fatalf("decode user settings: %v", err)
	}
	if request.SidebarTaskColorPatch == nil {
		t.Fatal("sidebar task color patch is nil")
	}
	if request.SidebarTaskColorPatch.Colors["task-1"] == nil ||
		*request.SidebarTaskColorPatch.Colors["task-1"] != "blue" {
		t.Fatalf("task-1 color = %#v", request.SidebarTaskColorPatch.Colors["task-1"])
	}
	if value, exists := request.SidebarTaskColorPatch.Colors["task-2"]; !exists || value != nil {
		t.Fatalf("task-2 color = %#v, want a clear tombstone", value)
	}
}

func TestAgentProfileCollectionsRedactEmbeddedEnvironmentValues(t *testing.T) {
	values := map[string]any{
		"profiles": []any{
			map[string]any{
				"name":     "Default",
				"env_vars": []any{map[string]any{"key": "TOKEN", "value": "secret-value", "secret_id": "secret-1"}},
			},
		},
	}
	redactAgentProfileCollections(values)

	profile := values["profiles"].([]any)[0].(map[string]any)
	entry := profile["env_vars"].([]any)[0].(map[string]any)
	if entry["value"] != "[redacted]" || entry["secret_id"] != "secret-1" {
		t.Fatalf("embedded environment value was not safely redacted: %#v", entry)
	}
}
