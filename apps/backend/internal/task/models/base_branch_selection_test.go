package models

import (
	"strings"
	"testing"
)

func TestBaseBranchSelectionRoundTripsAndPreservesMetadata(t *testing.T) {
	metadata := map[string]interface{}{"unrelated": "keep"}
	selection := BaseBranchSelection{
		Version: BaseBranchSelectionVersion,
		Branch:  "release/1.2",
		Source:  BaseBranchSelectionSourceInherited,
	}
	if err := PutBaseBranchSelection(metadata, &selection); err != nil {
		t.Fatalf("PutBaseBranchSelection: %v", err)
	}
	loaded, ok, err := LoadBaseBranchSelection(metadata)
	if err != nil || !ok {
		t.Fatalf("LoadBaseBranchSelection: ok=%v err=%v", ok, err)
	}
	if loaded != selection {
		t.Fatalf("loaded = %#v, want %#v", loaded, selection)
	}
	if metadata["unrelated"] != "keep" {
		t.Fatalf("unrelated metadata changed: %#v", metadata)
	}
}

func TestBaseBranchSelectionValidationFailsClosed(t *testing.T) {
	cases := []struct {
		name      string
		selection BaseBranchSelection
		want      string
	}{
		{
			name:      "unknown version",
			selection: BaseBranchSelection{Version: 2, Branch: "main", Source: BaseBranchSelectionSourceExplicit},
			want:      "version",
		},
		{
			name:      "unknown source",
			selection: BaseBranchSelection{Version: 1, Branch: "main", Source: "guessed"},
			want:      "source",
		},
		{
			name:      "explicit without branch",
			selection: BaseBranchSelection{Version: 1, Source: BaseBranchSelectionSourceExplicit},
			want:      "requires a branch",
		},
		{
			name:      "unsafe branch",
			selection: BaseBranchSelection{Version: 1, Branch: "../main", Source: BaseBranchSelectionSourceInherited},
			want:      "invalid",
		},
		{
			name:      "untrimmed branch",
			selection: BaseBranchSelection{Version: 1, Branch: " main", Source: BaseBranchSelectionSourceRepositoryDefault},
			want:      "trimmed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.selection.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestBaseBranchSelectionPolicy(t *testing.T) {
	cases := []struct {
		name       string
		metadata   map[string]interface{}
		baseBranch string
		attachment bool
		want       BaseBranchFallbackPolicy
	}{
		{
			name:       "missing metadata stays strict",
			metadata:   map[string]interface{}{},
			baseBranch: "feature/x",
			want:       BaseBranchFallbackPolicyStrict,
		},
		{
			name: "explicit selection stays strict",
			metadata: map[string]interface{}{BaseBranchSelectionMetadataKey: BaseBranchSelection{
				Version: 1, Branch: "feature/x", Source: BaseBranchSelectionSourceExplicit,
			}},
			baseBranch: "feature/x",
			want:       BaseBranchFallbackPolicyStrict,
		},
		{
			name: "explicit default name stays strict",
			metadata: map[string]interface{}{BaseBranchSelectionMetadataKey: BaseBranchSelection{
				Version: 1, Branch: "main", Source: BaseBranchSelectionSourceExplicit,
			}},
			baseBranch: "main",
			want:       BaseBranchFallbackPolicyStrict,
		},
		{
			name: "inherited selection permits recovery",
			metadata: map[string]interface{}{BaseBranchSelectionMetadataKey: BaseBranchSelection{
				Version: 1, Branch: "feature/x", Source: BaseBranchSelectionSourceInherited,
			}},
			baseBranch: "feature/x",
			want:       BaseBranchFallbackPolicyRepositoryDefault,
		},
		{
			name: "automatic default permits recovery",
			metadata: map[string]interface{}{BaseBranchSelectionMetadataKey: BaseBranchSelection{
				Version: 1, Branch: "main", Source: BaseBranchSelectionSourceRepositoryDefault,
			}},
			baseBranch: "main",
			want:       BaseBranchFallbackPolicyRepositoryDefault,
		},
		{
			name: "deferred default binding permits recovery",
			metadata: map[string]interface{}{BaseBranchSelectionMetadataKey: BaseBranchSelection{
				Version: 1, Source: BaseBranchSelectionSourceRepositoryDefault,
			}},
			baseBranch: "trunk",
			want:       BaseBranchFallbackPolicyRepositoryDefault,
		},
		{
			name: "stale branch binding stays strict",
			metadata: map[string]interface{}{BaseBranchSelectionMetadataKey: BaseBranchSelection{
				Version: 1, Branch: "main", Source: BaseBranchSelectionSourceRepositoryDefault,
			}},
			baseBranch: "release",
			want:       BaseBranchFallbackPolicyStrict,
		},
		{
			name: "manual override wins over permissive metadata",
			metadata: map[string]interface{}{
				BaseBranchSelectionMetadataKey: BaseBranchSelection{
					Version: 1, Branch: "main", Source: BaseBranchSelectionSourceInherited,
				},
				ManualBaseBranchOverrideMetadataKey: true,
			},
			baseBranch: "main",
			want:       BaseBranchFallbackPolicyStrict,
		},
		{
			name: "malformed record stays strict",
			metadata: map[string]interface{}{
				BaseBranchSelectionMetadataKey: map[string]interface{}{"version": 1, "branch": "main", "source": "guessed"},
			},
			baseBranch: "main",
			want:       BaseBranchFallbackPolicyStrict,
		},
		{
			name: "wrongly typed record stays strict",
			metadata: map[string]interface{}{
				BaseBranchSelectionMetadataKey: "inherited",
			},
			baseBranch: "main",
			want:       BaseBranchFallbackPolicyStrict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveBaseBranchFallbackPolicy(tc.metadata, tc.baseBranch)
			if got != tc.want {
				t.Fatalf("ResolveBaseBranchFallbackPolicy() = %q, want %q", got, tc.want)
			}
			if tc.attachment {
				if attachment := ResolveAttachmentBaseBranchFallbackPolicy(tc.metadata, tc.baseBranch); attachment != tc.want {
					t.Fatalf("ResolveAttachmentBaseBranchFallbackPolicy() = %q, want %q", attachment, tc.want)
				}
			}
		})
	}
}

func TestBaseBranchSelectionUnsetAttachmentBaseKeepsDefaultIntent(t *testing.T) {
	if got := ResolveAttachmentBaseBranchFallbackPolicy(map[string]interface{}{}, ""); got != BaseBranchFallbackPolicyRepositoryDefault {
		t.Fatalf("unset legacy base policy = %q, want repository default", got)
	}
	if got := ResolveAttachmentBaseBranchFallbackPolicy(map[string]interface{}{
		ManualBaseBranchOverrideMetadataKey: true,
	}, ""); got != BaseBranchFallbackPolicyStrict {
		t.Fatalf("manual override policy = %q, want strict", got)
	}
}
