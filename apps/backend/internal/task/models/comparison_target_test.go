package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func testComparisonTarget() ComparisonTarget {
	return ComparisonTarget{
		Version:      ComparisonTargetVersion,
		Provider:     ComparisonTargetProviderGitHub,
		Kind:         ComparisonTargetKindPullRequest,
		Number:       1154,
		HeadBranch:   "feature/cursor-cost",
		TargetBranch: "main",
		HeadRepository: ComparisonTargetRepository{
			Host:       "github.com",
			Path:       "contributor/widget",
			ProviderID: "head-42",
			RemoteURL:  "https://github.com/contributor/widget.git",
		},
		TargetRepository: ComparisonTargetRepository{
			Host:       "github.com",
			Path:       "upstream/widget",
			ProviderID: "base-99",
			RemoteURL:  "https://github.com/upstream/widget.git",
		},
	}
}

func TestComparisonTargetRoundTripsAndPreservesMetadata(t *testing.T) {
	target := testComparisonTarget()
	metadata := map[string]interface{}{"unrelated": "keep"}
	if err := PutComparisonTarget(metadata, &target); err != nil {
		t.Fatalf("PutComparisonTarget: %v", err)
	}

	got, ok, err := LoadComparisonTarget(metadata)
	if err != nil {
		t.Fatalf("LoadComparisonTarget: %v", err)
	}
	if !ok || !got.Equal(target) {
		t.Fatalf("loaded target = %#v, want %#v", got, target)
	}
	if metadata["unrelated"] != "keep" {
		t.Fatalf("unrelated metadata changed: %#v", metadata)
	}
}

func TestComparisonTargetValidationFailsClosed(t *testing.T) {
	base := testComparisonTarget()
	cases := []struct {
		name   string
		mutate func(*ComparisonTarget)
		want   string
	}{
		{name: "unknown version", mutate: func(v *ComparisonTarget) { v.Version = 99 }, want: "version"},
		{name: "unsafe target ref", mutate: func(v *ComparisonTarget) { v.TargetBranch = "../main" }, want: "target_branch"},
		{name: "missing head identity", mutate: func(v *ComparisonTarget) { v.HeadRepository.Path = "" }, want: "head_repository"},
		{name: "credential-bearing URL", mutate: func(v *ComparisonTarget) {
			v.TargetRepository.RemoteURL = "https://user:secret@github.com/upstream/widget.git"
		}, want: "credentials"},
		{name: "provider mismatch", mutate: func(v *ComparisonTarget) { v.TargetRepository.Host = "gitlab.example.test" }, want: "provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestComparisonTargetNamingAndDisplay(t *testing.T) {
	target := testComparisonTarget()
	if got := target.DisplayIdentity(); got != "upstream/widget:main" {
		t.Fatalf("DisplayIdentity() = %q", got)
	}
	if got := target.ComparisonRemoteName(); !strings.HasPrefix(got, "compare-") {
		t.Fatalf("ComparisonRemoteName() = %q, want compare- prefix", got)
	}
	if got := target.ComparisonRef(); !strings.HasPrefix(got, "refs/remotes/") || !strings.HasSuffix(got, "/main") {
		t.Fatalf("ComparisonRef() = %q, want exact remote-tracking shape", got)
	}
	other := target
	other.TargetRepository.Path = "another/widget"
	if target.ComparisonRemoteName() == other.ComparisonRemoteName() {
		t.Fatal("different target repositories share a comparison remote name")
	}
}

func TestComparisonTargetRemoveIsSourceAware(t *testing.T) {
	target := testComparisonTarget()
	metadata := map[string]interface{}{}
	if err := PutComparisonTarget(metadata, &target); err != nil {
		t.Fatal(err)
	}
	other := target
	other.Number++
	removed, err := RemoveComparisonTarget(metadata, &other)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("removed a target owned by another change")
	}
	removed, err = RemoveComparisonTarget(metadata, &target)
	if err != nil || !removed {
		t.Fatalf("RemoveComparisonTarget() = (%v, %v), want true and nil", removed, err)
	}
	if _, ok, err := LoadComparisonTarget(metadata); err != nil || ok {
		t.Fatalf("target remains after removal: ok=%v err=%v", ok, err)
	}
}

func TestComparisonTargetChangeIdentityIncludesTargetRepository(t *testing.T) {
	left := testComparisonTarget()
	right := left
	if !left.ChangeIdentityEqual(right) {
		t.Fatal("identical provider changes should have equal identities")
	}
	right.TargetRepository = ComparisonTargetRepository{
		Host:       "github.com",
		Path:       "another/widget",
		ProviderID: "base-100",
		RemoteURL:  "https://github.com/another/widget.git",
	}
	if left.ChangeIdentityEqual(right) {
		t.Fatal("same-number changes in different target repositories must differ")
	}
}

func TestComparisonTargetRejectsMalformedRehydratedMetadata(t *testing.T) {
	metadata := map[string]interface{}{ComparisonTargetMetadataKey: json.RawMessage(`{"version":1}`)}
	if _, ok, err := LoadComparisonTarget(metadata); err == nil || ok {
		t.Fatalf("LoadComparisonTarget() = (%v, %v), want error and no target", ok, err)
	}
}
