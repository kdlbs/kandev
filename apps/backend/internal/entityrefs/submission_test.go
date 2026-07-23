package entityrefs

import (
	"context"
	"errors"
	"testing"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeConversationResolver struct {
	sessionID      string
	assertedTaskID string
	workspaceID    string
	err            error
}

func (r *fakeConversationResolver) ResolveWorkspace(
	_ context.Context,
	sessionID, assertedTaskID string,
) (string, error) {
	r.sessionID = sessionID
	r.assertedTaskID = assertedTaskID
	return r.workspaceID, r.err
}

type fakeWorkspaceAuthorizer struct {
	workspaceID string
	references  []apiv1.EntityReference
	err         error
}

func (a *fakeWorkspaceAuthorizer) AuthorizeForWorkspace(
	_ context.Context,
	workspaceID string,
	reference apiv1.EntityReference,
) error {
	a.workspaceID = workspaceID
	a.references = append(a.references, reference)
	return a.err
}

func TestSubmissionServiceNormalizesAndAuthorizesConversationReferences(t *testing.T) {
	resolver := &fakeConversationResolver{workspaceID: "workspace-1"}
	authorizer := &fakeWorkspaceAuthorizer{}
	validator := NewSubmissionService(resolver, authorizer)
	reference := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("kandev", "task", "workspace-1", "task-1"),
		Provider: "kandev", Kind: "task", ID: "task-1",
		Title: " Task ", URL: "/t/task-1", Scope: "workspace-1",
	}

	got, err := validator.ValidateForSubmission(
		context.Background(), "session-1", "task-current", []apiv1.EntityReference{reference, reference},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if resolver.sessionID != "session-1" || resolver.assertedTaskID != "task-current" {
		t.Fatalf("resolver request = session %q task %q", resolver.sessionID, resolver.assertedTaskID)
	}
	if authorizer.workspaceID != "workspace-1" || len(authorizer.references) != 1 {
		t.Fatalf("authorization = workspace %q refs %#v", authorizer.workspaceID, authorizer.references)
	}
	if len(got) != 1 || got[0].Title != "Task" {
		t.Fatalf("normalized = %#v", got)
	}
}

func TestSubmissionServiceFailsClosed(t *testing.T) {
	valid := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      CanonicalRef("jira", "issue", "site", "100"),
		Provider: "jira", Kind: "issue", ID: "100", Title: "Issue",
		URL: "https://jira.test/browse/ENG-1", Scope: "site",
	}
	tests := []struct {
		name       string
		resolver   ConversationResolver
		authorizer WorkspaceAuthorizer
		reference  apiv1.EntityReference
	}{
		{name: "missing resolver", authorizer: &fakeWorkspaceAuthorizer{}, reference: valid},
		{name: "missing authorizer", resolver: &fakeConversationResolver{workspaceID: "workspace-1"}, reference: valid},
		{name: "resolver rejects", resolver: &fakeConversationResolver{err: errors.New("task mismatch")}, authorizer: &fakeWorkspaceAuthorizer{}, reference: valid},
		{name: "provider rejects", resolver: &fakeConversationResolver{workspaceID: "workspace-1"}, authorizer: &fakeWorkspaceAuthorizer{err: errors.New("origin mismatch")}, reference: valid},
		{name: "malformed", resolver: &fakeConversationResolver{workspaceID: "workspace-1"}, authorizer: &fakeWorkspaceAuthorizer{}, reference: apiv1.EntityReference{Version: 1, Ref: "bad"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validator := NewSubmissionService(test.resolver, test.authorizer)
			_, err := validator.ValidateForSubmission(context.Background(), "session", "task", []apiv1.EntityReference{test.reference})
			if err == nil {
				t.Fatal("validation succeeded")
			}
		})
	}
}

func TestSubmissionServiceRejectsOverLimitBeforeScopeOrProviderWork(t *testing.T) {
	resolver := &fakeConversationResolver{workspaceID: "workspace-1"}
	authorizer := &fakeWorkspaceAuthorizer{}
	validator := NewSubmissionService(resolver, authorizer)
	references := make([]apiv1.EntityReference, maxReferencesPerMessage+1)

	_, err := validator.ValidateForSubmission(
		context.Background(), "session-1", "task-1", references,
	)
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("error = %v, want invalid reference", err)
	}
	if resolver.sessionID != "" || len(authorizer.references) != 0 {
		t.Fatalf("scope/provider work occurred: resolver=%q refs=%d", resolver.sessionID, len(authorizer.references))
	}
}
