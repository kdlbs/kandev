package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type fakeAzureDevOpsCredentials struct {
	password     string
	err          error
	workspaceIDs []string
}

func (f *fakeAzureDevOpsCredentials) ResolveAzureDevOpsCloneCredentials(
	_ context.Context, workspaceID string,
) (string, string, error) {
	f.workspaceIDs = append(f.workspaceIDs, workspaceID)
	if f.err != nil {
		return "", "", f.err
	}
	return "kandev", f.password, nil
}

func TestEnsureClonedWithWorkspaceAuthFailsClosedForAzureCredentials(t *testing.T) {
	t.Parallel()
	azure := &models.Repository{
		WorkspaceID: "workspace-1", Provider: "azure_devops", ProviderOwner: "Platform", ProviderName: "api",
	}
	cloneURL := "https://dev.azure.com/acme/Platform/_git/api"

	unwired := &Executor{repoCloner: &recordingAuthenticatedCloner{}}
	if _, err := unwired.ensureClonedWithWorkspaceAuth(context.Background(), azure, cloneURL); err == nil {
		t.Fatal("clone without an Azure credential resolver must fail")
	}

	cloner := &recordingAuthenticatedCloner{}
	failing := &Executor{repoCloner: cloner}
	failing.SetAzureDevOpsCredentialResolver(&fakeAzureDevOpsCredentials{err: errors.New("sign in again")})
	_, err := failing.ensureClonedWithWorkspaceAuth(context.Background(), azure, cloneURL)
	if err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("resolver error = %v, want wrapped resolver failure", err)
	}
	if cloner.authCalls != 0 {
		t.Fatalf("clone ran %d times after credential failure", cloner.authCalls)
	}
}
