package lifecycle

import (
	"reflect"
	"testing"
)

// TestCollectBaseBranches_MultiRepo verifies the per-repo map populated for
// multi-repo launches: one entry per RepoLaunchSpec keyed by RepoName, plus
// the legacy unkeyed entry when LaunchRequest carries the singular top-level
// BaseBranch (for the synthesized single-repo path).
func TestCollectBaseBranches_MultiRepo(t *testing.T) {
	req := &LaunchRequest{
		Repositories: []RepoLaunchSpec{
			{RepoName: "alpha", BaseBranch: "main"},
			{RepoName: "beta", BaseBranch: "develop"},
		},
	}
	got := collectBaseBranches(req)
	want := map[string]string{"alpha": "main", "beta": "develop"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectBaseBranches = %v, want %v", got, want)
	}
}

// TestCollectBaseBranches_SingleRepoLegacyKey verifies the synthesized
// single-repo path: when only top-level BaseBranch is set, it lands under the
// empty key so the root WorkspaceTracker (repositoryName == "") finds it.
func TestCollectBaseBranches_SingleRepoLegacyKey(t *testing.T) {
	req := &LaunchRequest{
		RepositoryID:   "repo-1",
		RepositoryPath: "/tmp/repo",
		RepoName:       "repo",
		BaseBranch:     "main",
	}
	got := collectBaseBranches(req)
	want := map[string]string{"repo": "main", "": "main"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectBaseBranches = %v, want %v", got, want)
	}
}

// TestCollectBaseBranches_NoBaseBranchReturnsNil ensures we don't leak an
// empty map into metadata for tasks without recorded base branches.
func TestCollectBaseBranches_NoBaseBranchReturnsNil(t *testing.T) {
	req := &LaunchRequest{
		Repositories: []RepoLaunchSpec{
			{RepoName: "alpha"}, // no BaseBranch
		},
	}
	if got := collectBaseBranches(req); got != nil {
		t.Errorf("collectBaseBranches = %v, want nil", got)
	}
}

// TestCollectBaseBranches_RepoLessReturnsNil covers quick-chat / repo-less
// tasks. RepoSpecs returns nil, so the function must too — no metadata key
// gets injected for them.
func TestCollectBaseBranches_RepoLessReturnsNil(t *testing.T) {
	req := &LaunchRequest{}
	if got := collectBaseBranches(req); got != nil {
		t.Errorf("collectBaseBranches = %v, want nil", got)
	}
}

// TestGetMetadataStringMap_RoundTrips covers both shapes the metadata value
// can take: the in-process map[string]string written by collectBaseBranches,
// and the map[string]interface{} that results from a JSON round-trip (e.g.
// when metadata is persisted and restored on a session resume).
func TestGetMetadataStringMap_RoundTrips(t *testing.T) {
	t.Run("concrete map[string]string", func(t *testing.T) {
		md := map[string]interface{}{
			"base_branches": map[string]string{"repo-a": "main"},
		}
		got := getMetadataStringMap(md, "base_branches")
		if !reflect.DeepEqual(got, map[string]string{"repo-a": "main"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("JSON-decoded map[string]interface{}", func(t *testing.T) {
		md := map[string]interface{}{
			"base_branches": map[string]interface{}{"repo-a": "main", "repo-b": "develop"},
		}
		got := getMetadataStringMap(md, "base_branches")
		want := map[string]string{"repo-a": "main", "repo-b": "develop"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("missing key returns nil", func(t *testing.T) {
		if got := getMetadataStringMap(nil, "missing"); got != nil {
			t.Errorf("expected nil for nil metadata, got %v", got)
		}
		if got := getMetadataStringMap(map[string]interface{}{}, "missing"); got != nil {
			t.Errorf("expected nil for missing key, got %v", got)
		}
	})

	t.Run("non-string values are dropped", func(t *testing.T) {
		md := map[string]interface{}{
			"base_branches": map[string]interface{}{
				"repo-a": "main",
				"repo-b": 42, // intentionally wrong type
			},
		}
		got := getMetadataStringMap(md, "base_branches")
		want := map[string]string{"repo-a": "main"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// TestBuildLaunchMetadata_IncludesBaseBranches confirms the per-repo map is
// attached to metadata under MetadataKeyBaseBranches so executor call sites
// can forward it into CreateInstanceRequest.
func TestBuildLaunchMetadata_IncludesBaseBranches(t *testing.T) {
	req := &LaunchRequest{
		Repositories: []RepoLaunchSpec{
			{RepoName: "alpha", BaseBranch: "main"},
			{RepoName: "beta", BaseBranch: "develop"},
		},
	}
	md := buildLaunchMetadata(req, "", "", "")
	raw, ok := md[MetadataKeyBaseBranches]
	if !ok {
		t.Fatalf("metadata missing key %q", MetadataKeyBaseBranches)
	}
	got, ok := raw.(map[string]string)
	if !ok {
		t.Fatalf("metadata value type = %T, want map[string]string", raw)
	}
	want := map[string]string{"alpha": "main", "beta": "develop"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
