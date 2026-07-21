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

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/integrations/secretadapter"
	"github.com/kandev/kandev/internal/secrets"
)

func TestGitHubAppBootLoadsCatalogRegistrationFromEncryptedStore(t *testing.T) {
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
	repository := github.NewAppRegistrationRepository(seedService.TestStore(), adapter)
	registration := &github.AppRegistration{
		ID: "11111111-1111-4111-8111-111111111111", Source: github.AppRegistrationSourceManaged,
		DisplayName: "Kandev Acme", GitHubHost: "github.com", AppID: 123,
		ClientID: "Iv1.client", Slug: "kandev-acme",
		OwnerLogin: "acme", OwnerType: github.DeploymentAppOwnerOrganization,
		Visibility:    github.AppRegistrationVisibilityPrivate,
		PublicBaseURL: "https://kandev.example", CredentialGeneration: 1,
		Status: github.AppRegistrationStatusActive, WebhookStatus: github.DeploymentAppWebhookUnverified,
	}
	privateKey := testGitHubDeploymentPrivateKey(t)
	if err := repository.SaveRegistration(ctx, registration, github.DeploymentAppCredentials{
		PrivateKey: privateKey, ClientSecret: "client-secret",
		WebhookSecret: "webhook-secret",
	}); err != nil {
		t.Fatal(err)
	}
	const orphanID = github.AppRegistrationCredentialsSecretPrefix + "orphan:g99:secret"
	if err := adapter.Set(ctx, orphanID, "orphan", `{"version":1}`); err != nil {
		t.Fatal(err)
	}

	service := initGitHubService(
		&config.Config{}, db.NewPool(database, database), nil, secretStore, log,
	)
	if service == nil {
		t.Fatal("GitHub service was not initialized")
	}
	snapshot := service.AppRegistrationRuntimeSnapshot(registration.ID)
	if !snapshot.Ready || snapshot.Source != github.DeploymentAppSourceManaged ||
		snapshot.AppID != registration.AppID || snapshot.Generation != 1 {
		t.Fatalf("boot runtime = %+v", snapshot)
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
