package entityrefs

import (
	"encoding/json"
	"errors"
	"testing"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestNormalizeForSubmissionCanonicalizesAndDeduplicates(t *testing.T) {
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("jira", "issue", " site-a ", " 10001 "),
		Provider: " jira ",
		Kind:     " issue ",
		ID:       " 10001 ",
		Key:      " ENG-7\n",
		Title:    " Fix\n authentication ",
		URL:      " https://jira.example.test/browse/ENG-7 ",
		Scope:    " site-a ",
	}

	got, err := NormalizeForSubmission([]apiv1.EntityReference{ref, ref})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("references = %#v, want one deduplicated reference", got)
	}
	want := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("jira", "issue", "site-a", "10001"),
		Provider: "jira",
		Kind:     "issue",
		ID:       "10001",
		Key:      "ENG-7",
		Title:    "Fix authentication",
		URL:      "https://jira.example.test/browse/ENG-7",
		Scope:    "site-a",
	}
	if got[0] != want {
		t.Fatalf("reference = %#v, want %#v", got[0], want)
	}
}

func TestNormalizeForSubmissionRejectsMalformedKnownVersion(t *testing.T) {
	valid := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("kandev", "task", "workspace-1", "task-1"),
		Provider: "kandev", Kind: "task", ID: "task-1",
		Title: "Task", URL: "/t/task-1", Scope: "workspace-1",
	}
	tests := []struct {
		name string
		edit func(*apiv1.EntityReference)
	}{
		{name: "spoofed ref", edit: func(ref *apiv1.EntityReference) { ref.Ref = "mention:v1:kandev:task:workspace-1:other" }},
		{name: "unsafe scheme", edit: func(ref *apiv1.EntityReference) { ref.URL = "javascript:alert(1)" }},
		{name: "external internal route", edit: func(ref *apiv1.EntityReference) {
			ref.Provider = "jira"
			ref.Ref = CanonicalRef("jira", "task", ref.Scope, ref.ID)
		}},
		{name: "wrong task route", edit: func(ref *apiv1.EntityReference) { ref.URL = "/t/other" }},
		{name: "empty title", edit: func(ref *apiv1.EntityReference) { ref.Title = " \n " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := valid
			test.edit(&ref)
			_, err := NormalizeForSubmission([]apiv1.EntityReference{ref})
			if !errors.Is(err, ErrInvalidReference) {
				t.Fatalf("error = %v, want invalid reference", err)
			}
		})
	}
}

func TestNormalizeForSubmissionOmitsUnknownVersion(t *testing.T) {
	got, err := NormalizeForSubmission([]apiv1.EntityReference{{
		Version: 99, Ref: "future", Provider: "future", Kind: "thing", ID: "1",
		Title: "Future", URL: "https://future.test/1", Scope: "tenant",
	}})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("references = %#v, want unknown version omitted", got)
	}
}

func TestNormalizeForSubmissionDefaultsLegacyReferencesToContextOnly(t *testing.T) {
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("github", "pull_request", "acme/repo", "17"),
		Provider: "github", Kind: "pull_request", ID: "17", Key: "acme/repo#17",
		Title: "Fix auth", URL: "https://github.com/acme/repo/pull/17", Scope: "acme/repo",
	}
	got, err := NormalizeForSubmission([]apiv1.EntityReference{ref})
	if err != nil {
		t.Fatalf("normalize legacy reference: %v", err)
	}
	if len(got) != 1 || got[0].AssociationMode != "" || got[0].SelectionID != "" {
		t.Fatalf("legacy reference = %#v, want an omitted intent that remains context-only", got)
	}
}

func TestNormalizeForSubmissionRequiresSelectionIdentityForSupportedLinkIntent(t *testing.T) {
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("gitlab", "merge_request", "https://gitlab.example.test:2022/group/project", "7"),
		Provider: "gitlab", Kind: "merge_request", ID: "7", Key: "group/project!7",
		Title: "Fix auth", URL: "https://gitlab.example.test:2022/group/project/-/merge_requests/7",
		Scope: "https://gitlab.example.test:2022/group/project", SelectionID: "selection-1",
		AssociationMode: apiv1.EntityReferenceModeLinkOnSend,
	}
	got, err := NormalizeForSubmission([]apiv1.EntityReference{ref})
	if err != nil || len(got) != 1 || got[0].SelectionID != ref.SelectionID || got[0].AssociationMode != ref.AssociationMode {
		t.Fatalf("normalize link intent = %#v, err=%v", got, err)
	}

	withoutID := ref
	withoutID.SelectionID = ""
	if _, err := NormalizeForSubmission([]apiv1.EntityReference{withoutID}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("missing selection ID error = %v, want invalid reference", err)
	}
	unsupported := ref
	unsupported.Provider = "azure_devops"
	unsupported.Ref = CanonicalRef(unsupported.Provider, unsupported.Kind, unsupported.Scope, unsupported.ID)
	if _, err := NormalizeForSubmission([]apiv1.EntityReference{unsupported}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("unsupported provider intent error = %v, want invalid reference", err)
	}
}

func TestNormalizePersistedDecodesJSONRoundTripAndOmitsMalformed(t *testing.T) {
	valid := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("linear", "issue", "acme", "issue-1"),
		Provider: "linear", Kind: "issue", ID: "issue-1", Key: "ENG-1",
		Title: "Fix auth", URL: "https://linear.app/acme/issue/ENG-1", Scope: "acme",
	}
	rawJSON, err := json.Marshal([]apiv1.EntityReference{valid, {
		Version: apiv1.EntityReferenceVersion,
		Ref:     "bad", Provider: "linear", Kind: "issue", ID: "bad",
		Title: "Bad", URL: "https://linear.app/bad", Scope: "acme",
	}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTripped any
	if err := json.Unmarshal(rawJSON, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := NormalizePersisted(roundTripped)
	if len(got) != 1 || got[0] != valid {
		t.Fatalf("references = %#v, want only valid typed reference", got)
	}
}
