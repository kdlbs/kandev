package gitcredentials

import (
	"strings"
	"testing"
)

func TestResolveRepositoryIdentityForAzureReposCloneURL(t *testing.T) {
	input := RepositoryIdentityInput{
		RepositoryID:  "repo-1",
		Provider:      "azure_devops",
		ProviderHost:  "https://dev.azure.com",
		ProviderOwner: "Contoso",
		ProviderName:  "api",
		RemoteURL:     "https://dev.azure.com/acme/Contoso/_git/api",
	}
	identity, err := ResolveRepositoryIdentity(input)
	if err != nil {
		t.Fatalf("ResolveRepositoryIdentity: %v", err)
	}
	if identity.Host != "dev.azure.com" || identity.Path != "/acme/Contoso/_git/api" {
		t.Fatalf("identity = %+v, want dev.azure.com /acme/Contoso/_git/api", identity)
	}

	input.ProviderHost = ""
	if _, err := ResolveRepositoryIdentity(input); err == nil || !strings.Contains(err.Error(), "needs a provider host") {
		t.Fatalf("without a provider host err = %v, want the provider host error", err)
	}
}
