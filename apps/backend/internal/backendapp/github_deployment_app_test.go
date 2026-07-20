package backendapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/integrations/secretadapter"
	"github.com/kandev/kandev/internal/secrets"
)

func TestGitHubDeploymentAppBootLoadsManagedRegistrationFromEncryptedStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := db.OpenSQLite(filepath.Join(dir, "kandev.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	masterKey, err := secrets.NewMasterKeyProvider(dir)
	if err != nil {
		t.Fatal(err)
	}
	secretStore, cleanup, err := secrets.Provide(database, database, masterKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	seedService, cleanupGitHub, err := github.Provide(database, database, nil, nil, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupGitHub() })
	adapter := secretadapter.New(secretStore)
	repository := github.NewDeploymentAppRepository(seedService.TestStore(), adapter)
	registration := &github.DeploymentAppRegistration{
		GitHubHost: "github.com", AppID: 123, ClientID: "Iv1.client", Slug: "kandev-acme",
		OwnerLogin: "acme", OwnerType: github.DeploymentAppOwnerOrganization,
		PublicBaseURL: "https://kandev.example", CredentialGeneration: 1,
		WebhookStatus: github.DeploymentAppWebhookUnverified,
	}
	privateKey := testGitHubDeploymentPrivateKey(t)
	if err := repository.SaveManagedRegistration(ctx, registration, github.DeploymentAppCredentials{
		PrivateKey: privateKey, ClientSecret: "client-secret",
		WebhookSecret: "webhook-secret",
	}); err != nil {
		t.Fatal(err)
	}
	flow := &github.DeploymentAppRegistrationFlow{
		StateHash: "pending-flow", OperatorUserID: github.DefaultUserID,
		OwnerType: github.DeploymentAppOwnerOrganization, OwnerLogin: "acme",
		PublicBaseURL: "https://kandev.example", ManifestRevision: 1,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := seedService.TestStore().CreateDeploymentAppRegistrationFlow(ctx, flow); err != nil {
		t.Fatal(err)
	}
	const orphanID = "github:deployment-app:credentials:g99:orphan"
	if err := adapter.Set(ctx, orphanID, "orphan", `{"version":1}`); err != nil {
		t.Fatal(err)
	}

	service := initGitHubService(
		&config.Config{}, db.NewPool(database, database), nil, secretStore, log,
	)
	if service == nil {
		t.Fatal("GitHub service was not initialized")
	}
	snapshot := service.DeploymentAppRuntimeSnapshot()
	if !snapshot.Ready || snapshot.Source != github.DeploymentAppSourceManaged ||
		snapshot.AppID != registration.AppID || snapshot.Generation != 1 {
		t.Fatalf("boot runtime = %+v", snapshot)
	}
	status, err := service.DeploymentAppRegistrationStatus(ctx, github.DefaultUserID)
	if err != nil || !status.Ready || status.Registration == nil ||
		status.Registration.AppID != registration.AppID {
		t.Fatalf("registration status = %+v, error = %v", status, err)
	}
	if exists, err := adapter.Exists(ctx, orphanID); err != nil || exists {
		t.Fatalf("orphan credential exists = %v, error = %v", exists, err)
	}
	var encrypted []byte
	if err := database.Get(&encrypted, `SELECT encrypted_value FROM secrets WHERE id = ?`,
		registration.CredentialSecretID); err != nil {
		t.Fatal(err)
	}
	for _, plaintext := range [][]byte{[]byte("client-secret"), []byte("webhook-secret"), []byte(privateKey)} {
		if bytes.Contains(encrypted, plaintext) {
			t.Fatal("deployment credentials were stored as plaintext")
		}
	}
	activeSecretID := registration.CredentialSecretID
	if err := resetGitHubDeploymentAppForE2E(ctx, service); err != nil {
		t.Fatalf("reset deployment App: %v", err)
	}
	storedRegistration, err := service.TestStore().GetDeploymentAppRegistration(ctx)
	if err != nil || storedRegistration != nil {
		t.Fatalf("registration after reset = %+v, error = %v", storedRegistration, err)
	}
	storedFlow, err := service.TestStore().GetDeploymentAppRegistrationFlow(ctx, flow.StateHash)
	if err != nil || storedFlow != nil {
		t.Fatalf("flow after reset = %+v, error = %v", storedFlow, err)
	}
	for _, secretID := range []string{activeSecretID, orphanID} {
		if exists, err := adapter.Exists(ctx, secretID); err != nil || exists {
			t.Fatalf("secret %q after reset exists = %v, error = %v", secretID, exists, err)
		}
	}
	if snapshot := service.DeploymentAppRuntimeSnapshot(); snapshot.Ready {
		t.Fatalf("runtime remained ready after reset: %+v", snapshot)
	}
}

func testGitHubDeploymentPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}
