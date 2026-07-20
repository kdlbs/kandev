package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestDeploymentAppConfigSourcePrecedence(t *testing.T) {
	store := newTestStore(t)
	repository := NewDeploymentAppRepository(store, newFakeConnectionSecrets())
	managedCredentials := DeploymentAppCredentials{
		PrivateKey: testDeploymentAppPrivateKey(t), ClientSecret: "managed-client",
		WebhookSecret: "managed-webhook",
	}
	if err := repository.SaveManagedRegistration(
		context.Background(), testDeploymentAppRegistration(1), managedCredentials,
	); err != nil {
		t.Fatal(err)
	}

	partialEnvironment := config.GitHubAppConfig{ClientSecret: "environment-client"}
	resolved, err := ResolveDeploymentAppConfig(context.Background(), partialEnvironment, repository)
	if err == nil {
		t.Fatal("partial environment config must fail closed")
	}
	if resolved.Source != DeploymentAppSourceEnvironment ||
		resolved.Config.ClientSecret != partialEnvironment.ClientSecret {
		t.Fatalf("partial environment resolution = %+v", resolved)
	}

	completeEnvironment := testEnvironmentDeploymentAppConfig(t)
	completeEnvironment.AppID = 999
	resolved, err = ResolveDeploymentAppConfig(context.Background(), completeEnvironment, repository)
	if err != nil {
		t.Fatalf("resolve complete environment config: %v", err)
	}
	if resolved.Source != DeploymentAppSourceEnvironment || resolved.Config.AppID != 999 {
		t.Fatalf("environment resolution = %+v", resolved)
	}

	resolved, err = ResolveDeploymentAppConfig(context.Background(), config.GitHubAppConfig{}, repository)
	if err != nil {
		t.Fatalf("resolve managed config: %v", err)
	}
	if resolved.Source != DeploymentAppSourceManaged || resolved.Config.AppID != 123 ||
		resolved.Config.ClientSecret != managedCredentials.ClientSecret {
		t.Fatalf("managed resolution = %+v", resolved)
	}
}

func TestDeploymentAppConfigSourceNone(t *testing.T) {
	repository := NewDeploymentAppRepository(newTestStore(t), newFakeConnectionSecrets())
	resolved, err := ResolveDeploymentAppConfig(
		context.Background(), config.GitHubAppConfig{}, repository,
	)
	if err != nil {
		t.Fatalf("resolve absent deployment App config: %v", err)
	}
	if resolved.Source != DeploymentAppSourceNone || resolved.Config.Configured() {
		t.Fatalf("absent resolution = %+v", resolved)
	}
}

func testEnvironmentDeploymentAppConfig(t *testing.T) config.GitHubAppConfig {
	t.Helper()
	return config.GitHubAppConfig{
		AppID: 1, ClientID: "Iv1.environment", ClientSecret: "environment-client",
		PrivateKey: testDeploymentAppPrivateKey(t), WebhookSecret: "environment-webhook",
		Slug: "environment-app", PublicBaseURL: "https://kandev.example.com",
	}
}

func testDeploymentAppPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate deployment App private key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}
