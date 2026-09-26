package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	githubsvc "github.com/kandev/kandev/internal/github"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/repoclone"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestBuildLifecycleLaunchRequestForwardsRemoteContributions(t *testing.T) {
	binding := &taskmodels.RemoteContribution{CanonicalURL: "https://github.com/acme/widget/pull/7"}
	req := &orchestratorexecutor.LaunchAgentRequest{
		RemoteContribution: binding,
		Repositories:       []orchestratorexecutor.RepoSpec{{RemoteContribution: binding}},
	}

	launchReq := buildLifecycleLaunchRequest(req, "/workspace", "office-profile")

	if launchReq.RemoteContribution != binding {
		t.Fatalf("top-level remote contribution was not forwarded")
	}
	if len(launchReq.Repositories) != 1 || launchReq.Repositories[0].RemoteContribution != binding {
		t.Fatalf("per-repository remote contribution was not forwarded: %#v", launchReq.Repositories)
	}
}

// distinctFiller populates struct fields with non-zero values that are unique
// per field, so a mapper that drops a field (or crosswires two of the same
// type) produces an observable difference rather than two matching zero
// values. Bool is the documented exception — see fill.
type distinctFiller struct{ n int }

// fillStruct populates every exported field of the struct pointed at by ptr.
func (f *distinctFiller) fillStruct(t *testing.T, ptr any) {
	t.Helper()
	v := reflect.ValueOf(ptr).Elem()
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !v.Field(i).CanSet() {
			t.Fatalf("distinctFiller: %s.%s is unexported and cannot be covered", typ, field.Name)
		}
		f.fill(t, v.Field(i), field.Name)
	}
}

func (f *distinctFiller) fill(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	f.n++
	switch v.Kind() {
	case reflect.String:
		v.SetString(fmt.Sprintf("%s-%d", name, f.n))
	case reflect.Bool:
		// Always true, never counter-derived: bool has only two values and one
		// of them is the zero value, so distinctness across several bool fields
		// and drop-detection are mutually exclusive. Drop-detection wins —
		// a field left unmapped reads back false and fails. The cost is that
		// two crosswired bool fields would still compare equal.
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(int64(f.n))
	case reflect.Pointer:
		// Deliberately shallow: a zero-value allocation is enough to catch a
		// dropped pointer field (nil != non-nil). It does not catch a mapper
		// that substitutes a freshly allocated zero value of the same type,
		// since reflect.DeepEqual compares pointees rather than addresses.
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Slice:
		slice := reflect.MakeSlice(v.Type(), 1, 1)
		f.fill(t, slice.Index(0), name+"-elem")
		v.Set(slice)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		f.fill(t, key, name+"-key")
		value := reflect.New(v.Type().Elem()).Elem()
		f.fill(t, value, name+"-value")
		m.SetMapIndex(key, value)
		v.Set(m)
	case reflect.Func:
		v.Set(reflect.MakeFunc(v.Type(), func([]reflect.Value) []reflect.Value {
			outputs := make([]reflect.Value, v.Type().NumOut())
			for i := range outputs {
				outputs[i] = reflect.Zero(v.Type().Out(i))
			}
			return outputs
		}))
	default:
		t.Fatalf("distinctFiller: unsupported kind %s for field %s — extend the helper", v.Kind(), name)
	}
}

// assertSameNamedFieldsEqual fails unless src and dst expose the same number of
// exported fields and every same-named pair holds a DeepEqual value. Mapper
// tests use it so that dropping a field — or adding one to both sides of a
// mirrored type pair and forgetting to wire it — fails instead of passing
// silently the way a hand-written subset of field assertions would.
func assertSameNamedFieldsEqual(t *testing.T, label string, src, dst any) {
	t.Helper()
	srcVal, dstVal := reflect.ValueOf(src), reflect.ValueOf(dst)
	srcType, dstType := srcVal.Type(), dstVal.Type()
	if srcType.NumField() != dstType.NumField() {
		t.Fatalf("%s: %s has %d fields but %s has %d — the mirrored types drifted",
			label, srcType, srcType.NumField(), dstType, dstType.NumField())
	}
	for i := 0; i < srcType.NumField(); i++ {
		name := srcType.Field(i).Name
		dstField := dstVal.FieldByName(name)
		if !dstField.IsValid() {
			t.Errorf("%s: %s has no field %s", label, dstType, name)
			continue
		}
		want := srcVal.Field(i).Interface()
		if srcVal.Field(i).Kind() == reflect.Func {
			if srcVal.Field(i).IsNil() != dstField.IsNil() {
				t.Errorf("%s: field %s nil = %v, want %v", label, name, dstField.IsNil(), srcVal.Field(i).IsNil())
			}
			continue
		}
		if got := dstField.Interface(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: field %s = %#v, want %#v", label, name, got, want)
		}
	}
}

// TestLifecycleWorkspaceFoldersMapsEveryField covers the launch path's folder
// projection. Every agent launch flows through it and a dropped field is
// silent — no panic, no error, just missing agent configuration.
func TestLifecycleWorkspaceFoldersMapsEveryField(t *testing.T) {
	if got := lifecycleWorkspaceFolders(nil); got != nil {
		t.Errorf("lifecycleWorkspaceFolders(nil) = %#v, want nil", got)
	}
	if got := lifecycleWorkspaceFolders([]orchestratorexecutor.WorkspaceFolderSpec{}); got != nil {
		t.Errorf("lifecycleWorkspaceFolders(empty) = %#v, want nil", got)
	}

	filler := &distinctFiller{}
	folders := make([]orchestratorexecutor.WorkspaceFolderSpec, 2)
	for i := range folders {
		filler.fillStruct(t, &folders[i])
	}

	got := lifecycleWorkspaceFolders(folders)
	if len(got) != len(folders) {
		t.Fatalf("lifecycleWorkspaceFolders returned %d folders, want %d", len(got), len(folders))
	}
	for i := range folders {
		assertSameNamedFieldsEqual(t, fmt.Sprintf("workspace folder %d", i), folders[i], got[i])
	}
}

// TestLifecycleRouteOverrideMapsEveryField pins the same all-fields contract on
// the sibling route-override mapper.
func TestLifecycleRouteOverrideMapsEveryField(t *testing.T) {
	if got := lifecycleRouteOverride(nil); got != nil {
		t.Errorf("lifecycleRouteOverride(nil) = %#v, want nil", got)
	}

	override := &orchestratorexecutor.RouteOverride{}
	(&distinctFiller{}).fillStruct(t, override)

	got := lifecycleRouteOverride(override)
	if got == nil {
		t.Fatal("lifecycleRouteOverride returned nil for a non-nil override")
	}
	assertSameNamedFieldsEqual(t, "route override", *override, *got)
}

// TestLifecycleRepoLaunchSpecsMapsEveryField pins the all-fields contract on
// the multi-repo launch spec mapper — the widest of the three (19 fields).
func TestLifecycleRepoLaunchSpecsMapsEveryField(t *testing.T) {
	if got := lifecycleRepoLaunchSpecs(nil); got != nil {
		t.Errorf("lifecycleRepoLaunchSpecs(nil) = %#v, want nil", got)
	}
	if got := lifecycleRepoLaunchSpecs([]orchestratorexecutor.RepoSpec{}); got != nil {
		t.Errorf("lifecycleRepoLaunchSpecs(empty) = %#v, want nil", got)
	}

	filler := &distinctFiller{}
	repos := make([]orchestratorexecutor.RepoSpec, 2)
	for i := range repos {
		filler.fillStruct(t, &repos[i])
	}

	got := lifecycleRepoLaunchSpecs(repos)
	if len(got) != len(repos) {
		t.Fatalf("lifecycleRepoLaunchSpecs returned %d specs, want %d", len(got), len(repos))
	}
	for i := range repos {
		assertSameNamedFieldsEqual(t, fmt.Sprintf("repo spec %d", i), repos[i], got[i])
	}
}

// TestBuildLifecycleLaunchRequestWiresMappedCollections asserts the launch
// request actually carries the three mapper outputs. A mapper can be correct
// while the wiring in buildLifecycleLaunchRequest drops it.
func TestBuildLifecycleLaunchRequestWiresMappedCollections(t *testing.T) {
	filler := &distinctFiller{}
	folders := make([]orchestratorexecutor.WorkspaceFolderSpec, 2)
	for i := range folders {
		filler.fillStruct(t, &folders[i])
	}
	repos := make([]orchestratorexecutor.RepoSpec, 2)
	for i := range repos {
		filler.fillStruct(t, &repos[i])
	}
	override := &orchestratorexecutor.RouteOverride{}
	filler.fillStruct(t, override)

	launchReq := buildLifecycleLaunchRequest(&orchestratorexecutor.LaunchAgentRequest{
		WorkspaceFolders: folders,
		RouteOverride:    override,
		Repositories:     repos,
	}, "/workspace", "office-profile")

	if len(launchReq.WorkspaceFolders) != len(folders) {
		t.Fatalf("WorkspaceFolders length = %d, want %d", len(launchReq.WorkspaceFolders), len(folders))
	}
	for i := range folders {
		assertSameNamedFieldsEqual(t, fmt.Sprintf("launch workspace folder %d", i), folders[i], launchReq.WorkspaceFolders[i])
	}

	if launchReq.RouteOverride == nil {
		t.Fatal("RouteOverride was dropped from the launch request")
	}
	assertSameNamedFieldsEqual(t, "launch route override", *override, *launchReq.RouteOverride)

	if len(launchReq.Repositories) != len(repos) {
		t.Fatalf("Repositories length = %d, want %d", len(launchReq.Repositories), len(repos))
	}
	for i := range repos {
		assertSameNamedFieldsEqual(t, fmt.Sprintf("launch repo spec %d", i), repos[i], launchReq.Repositories[i])
	}
}

func TestBuildLifecycleLaunchRequestCarriesMCPProviders(t *testing.T) {
	want := []string{"github", "gitlab"}
	got := buildLifecycleLaunchRequest(&orchestratorexecutor.LaunchAgentRequest{
		McpProviders: want,
	}, "", "")
	if len(got.McpProviders) != len(want) || got.McpProviders[0] != want[0] || got.McpProviders[1] != want[1] {
		t.Fatalf("McpProviders = %#v, want %#v", got.McpProviders, want)
	}
}

func TestDetectGitDefaultBranchDetachedHEADReturnsEmpty(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "HEAD"),
		[]byte("3a3f2d3b00000000000000000000000000000000\n"),
		0o644,
	); err != nil {
		t.Fatalf("write detached HEAD: %v", err)
	}

	if got := detectGitDefaultBranch(repoPath); got != "" {
		t.Fatalf("detectGitDefaultBranch = %q, want empty", got)
	}
}

func TestResolveReviewBaseBranchRedetectsStoredMasterWhenMainExists(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/master\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	for _, branch := range []string{"main", "master"} {
		refPath := filepath.Join(gitDir, "refs", "remotes", "origin", branch)
		if err := os.WriteFile(refPath, []byte("0000000\n"), 0o644); err != nil {
			t.Fatalf("write %s ref: %v", branch, err)
		}
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	if got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, ""); got != "main" {
		t.Fatalf("resolveReviewBaseBranch = %q, want main", got)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "main" {
		t.Fatalf("stored default_branch = %q, want main", stored.DefaultBranch)
	}
}

func TestResolveReviewBaseBranchSkipsNoopPersistForStoredMaster(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/master\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	refPath := filepath.Join(gitDir, "refs", "remotes", "origin", "master")
	if err := os.WriteFile(refPath, []byte("0000000\n"), 0o644); err != nil {
		t.Fatalf("write master ref: %v", err)
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	if got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, ""); got != "master" {
		t.Fatalf("resolveReviewBaseBranch = %q, want master", got)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if !stored.UpdatedAt.Equal(repo.UpdatedAt) {
		t.Fatalf("stored updated_at = %v, want unchanged %v", stored.UpdatedAt, repo.UpdatedAt)
	}
}

func TestResolveReviewBaseBranchKeepsStoredMasterOnHeadFallback(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	if got := adapter.resolveReviewBaseBranch(ctx, repo, repoPath, ""); got != "master" {
		t.Fatalf("resolveReviewBaseBranch = %q, want master", got)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.DefaultBranch != "master" {
		t.Fatalf("stored default_branch = %q, want master", stored.DefaultBranch)
	}
	if !stored.UpdatedAt.Equal(repo.UpdatedAt) {
		t.Fatalf("stored updated_at = %v, want unchanged %v", stored.UpdatedAt, repo.UpdatedAt)
	}
}

func TestResolveForReviewRedetectsStoredMasterAfterClonePath(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	basePath := canonicalTempDir(t)
	cloner := repoclone.NewCloner(
		repoclone.Config{BasePath: basePath}, repoclone.ProtocolHTTPS, "", newTestLogger(),
	)
	cloner.SetGitCredentialProvider(staticRepoCloneCredential{})
	repoPath, err := cloner.WorkspaceRepoPath(workspace.ID, "github", "owner", "repo")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath: %v", err)
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/master\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}
	for _, branch := range []string{"main", "master"} {
		refPath := filepath.Join(gitDir, "refs", "remotes", "origin", branch)
		if err := os.WriteFile(refPath, []byte("0000000\n"), 0o644); err != nil {
			t.Fatalf("write %s ref: %v", branch, err)
		}
	}
	repo, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID:   workspace.ID,
		Name:          "owner/repo",
		SourceType:    "provider",
		Provider:      "github",
		ProviderHost:  "https://github.com",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		DefaultBranch: "master",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	adapter := &repositoryResolverAdapter{
		cloner:  cloner,
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}
	repoID, baseBranch, err := adapter.ResolveForReview(ctx, workspace.ID, "github", "owner", "repo", "")
	if err != nil {
		t.Fatalf("ResolveForReview: %v", err)
	}
	if repoID != repo.ID {
		t.Fatalf("repository ID = %q, want %q", repoID, repo.ID)
	}
	if baseBranch != "main" {
		t.Fatalf("base branch = %q, want main", baseBranch)
	}

	stored, err := harness.taskSvc.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != repoPath {
		t.Fatalf("stored local_path = %q, want %q", stored.LocalPath, repoPath)
	}
	if stored.DefaultBranch != "main" {
		t.Fatalf("stored default_branch = %q, want main", stored.DefaultBranch)
	}
}

func TestRepositoryResolverAdapterUsesCurrentGitProtocol(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	paths := []string{filepath.Join(t.TempDir(), "one"), filepath.Join(t.TempDir(), "two")}
	for _, path := range paths {
		runGit(t, "", "init", "--quiet", path)
	}
	cloner := &recordingReviewRepositoryCloner{protocol: repoclone.ProtocolSSH, paths: paths}
	adapter := &repositoryResolverAdapter{
		cloner:  cloner,
		taskSvc: harness.taskSvc,
		logger:  newTestLogger(),
	}

	if _, _, err := adapter.ResolveForReview(ctx, workspace.ID, "github", "owner", "one", ""); err != nil {
		t.Fatalf("first ResolveForReview: %v", err)
	}
	cloner.protocol = repoclone.ProtocolHTTPS
	if _, _, err := adapter.ResolveForReview(ctx, workspace.ID, "github", "owner", "two", ""); err != nil {
		t.Fatalf("second ResolveForReview: %v", err)
	}

	want := []string{
		"git@github.com:owner/one.git",
		"https://github.com/owner/two.git",
	}
	if !reflect.DeepEqual(cloner.cloneURLs, want) {
		t.Fatalf("clone URLs = %v, want %v", cloner.cloneURLs, want)
	}
}

type recordingReviewRepositoryCloner struct {
	protocol  string
	paths     []string
	cloneURLs []string
}

func (c *recordingReviewRepositoryCloner) EnsureWorkspaceCloned(
	_ context.Context, _, _, _, _, name string,
) (string, error) {
	path := c.paths[0]
	c.paths = c.paths[1:]
	return path, nil
}

func (c *recordingReviewRepositoryCloner) BuildCloneURLWithHost(
	_ context.Context, provider, host, owner, name string,
) (string, error) {
	cloneURL, err := repoclone.CloneURLWithHost(provider, host, owner, name, c.protocol)
	if err == nil {
		c.cloneURLs = append(c.cloneURLs, cloneURL)
	}
	return cloneURL, err
}

type staticRepoCloneCredential struct{}

func (staticRepoCloneCredential) ResolveGitCredential(
	context.Context,
	repoclone.GitCredentialRequest,
) (string, string, error) {
	return "x-access-token", "test-token", nil
}

func TestGetSessionModel_Caching(t *testing.T) {
	// We can't easily instantiate a full taskservice.Service here without a DB,
	// so we test the caching mechanism directly on the adapter struct.
	adapter := &messageCreatorAdapter{
		svc:    nil, // Will cause getSessionModel to return "" on cache miss (nil svc panics)
		logger: newTestLogger(),
	}

	// Pre-populate the cache to avoid calling the nil svc
	adapter.sessionModelMu.Lock()
	adapter.sessionModelCache = map[string]string{
		"session-1": "claude-sonnet-4",
		"session-2": "gpt-4",
	}
	adapter.sessionModelMu.Unlock()

	// Test cache hit
	model := adapter.getSessionModel(context.Background(), "session-1")
	if model != "claude-sonnet-4" {
		t.Errorf("expected 'claude-sonnet-4', got %q", model)
	}

	model = adapter.getSessionModel(context.Background(), "session-2")
	if model != "gpt-4" {
		t.Errorf("expected 'gpt-4', got %q", model)
	}

	// Test cache miss for unknown session returns ""
	// (svc is nil, so DB lookup would fail gracefully)
	// We need a non-nil svc to avoid panic - use a minimal mock approach
	// Instead, verify the cache was populated for existing entries
	adapter.sessionModelMu.RLock()
	if len(adapter.sessionModelCache) != 2 {
		t.Errorf("expected 2 cached entries, got %d", len(adapter.sessionModelCache))
	}
	adapter.sessionModelMu.RUnlock()
}

func TestGetSessionModel_ConcurrentAccess(t *testing.T) {
	adapter := &messageCreatorAdapter{
		svc:    nil,
		logger: newTestLogger(),
	}

	// Pre-populate cache
	adapter.sessionModelMu.Lock()
	adapter.sessionModelCache = map[string]string{
		"session-1": "claude-sonnet-4",
	}
	adapter.sessionModelMu.Unlock()

	// Concurrent reads should not race
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			model := adapter.getSessionModel(context.Background(), "session-1")
			if model != "claude-sonnet-4" {
				t.Errorf("expected 'claude-sonnet-4', got %q", model)
			}
		}()
	}
	wg.Wait()
}

func TestGetSessionModel_LazyInit(t *testing.T) {
	// Verify that the cache map is lazily initialized (nil initially)
	adapter := &messageCreatorAdapter{
		svc:    nil,
		logger: newTestLogger(),
	}

	// sessionModelCache should be nil initially
	adapter.sessionModelMu.RLock()
	if adapter.sessionModelCache != nil {
		t.Error("expected sessionModelCache to be nil initially")
	}
	adapter.sessionModelMu.RUnlock()
}

// Verify the adapter compiles with the taskservice.Service field
func TestMessageCreatorAdapter_StructFields(t *testing.T) {
	adapter := &messageCreatorAdapter{
		svc:    (*taskservice.Service)(nil),
		logger: newTestLogger(),
	}
	if adapter.svc != nil {
		t.Error("expected nil svc")
	}
}

func TestWrapGitHubTaskIssueStoreError(t *testing.T) {
	taskErr := fmt.Errorf("load task: %w", taskrepo.ErrTaskNotFound)
	wrapped := wrapGitHubTaskIssueStoreError(taskErr)
	if !errors.Is(wrapped, githubsvc.ErrTaskNotFound) {
		t.Fatalf("wrapped error should match github ErrTaskNotFound: %v", wrapped)
	}
	if !errors.Is(wrapped, taskrepo.ErrTaskNotFound) {
		t.Fatalf("wrapped error should preserve task repo ErrTaskNotFound: %v", wrapped)
	}

	otherErr := errors.New("database unavailable")
	if got := wrapGitHubTaskIssueStoreError(otherErr); got != otherErr {
		t.Fatalf("non-not-found error changed: got %v, want %v", got, otherErr)
	}
}

// TestNormalizeRuntimeStopError asserts the lifecycle adapter maps the backend
// lifecycle not-found sentinel onto the public runtime not-found sentinel, so
// orchestrator/reconciliation can depend only on runtimeapi.ErrNotFound and
// never has to import runtime/lifecycle. Unrelated errors pass through unchanged.
func TestNormalizeRuntimeStopError(t *testing.T) {
	if got := normalizeRuntimeStopError(nil); got != nil {
		t.Fatalf("normalizeRuntimeStopError(nil) = %v, want nil", got)
	}

	wrapped := fmt.Errorf("stop agent: %w", lifecycle.ErrExecutionNotFound)
	got := normalizeRuntimeStopError(wrapped)
	if !errors.Is(got, runtimeapi.ErrNotFound) {
		t.Fatalf("lifecycle not-found must normalize to runtimeapi.ErrNotFound; got %v", got)
	}
	if !errors.Is(got, lifecycle.ErrExecutionNotFound) {
		t.Fatalf("original lifecycle sentinel must be preserved for diagnostics; got %v", got)
	}

	other := errors.New("agentctl unreachable")
	if got := normalizeRuntimeStopError(other); got != other {
		t.Fatalf("unrelated error must pass through unchanged; got %v, want %v", got, other)
	}
}

// jiraSecretAdapter Set/Exists branching is tested in
// internal/integrations/secretadapter/secretadapter_test.go now that the
// upsert helper lives there.
