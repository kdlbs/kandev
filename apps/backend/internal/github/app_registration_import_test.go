package github

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type appRegistrationVerifierFunc func(context.Context) (AuthenticatedApp, error)

func (f appRegistrationVerifierFunc) GetAuthenticatedApp(ctx context.Context) (AuthenticatedApp, error) {
	return f(ctx)
}

type appRegistrationVerifierWithWebhook struct {
	app    AuthenticatedApp
	config AppWebhookConfig
}

func (v appRegistrationVerifierWithWebhook) GetAuthenticatedApp(context.Context) (AuthenticatedApp, error) {
	return v.app, nil
}

func (v appRegistrationVerifierWithWebhook) GetWebhookConfig(context.Context) (AppWebhookConfig, error) {
	return v.config, nil
}

func TestAppRegistrationImportReturnsKnownDuplicateWithoutVerification(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	request := validAppRegistrationImportRequest(t)
	existing := newAppRegistration("existing-registration", request.AppID, "Existing", time.Now().UTC())
	if err := repository.SaveRegistration(context.Background(), existing, DeploymentAppCredentials{
		PrivateKey: "existing-key", ClientSecret: "existing-client", WebhookSecret: "existing-webhook",
	}); err != nil {
		t.Fatal(err)
	}
	importer := NewAppRegistrationImporter(repository, importPublicResolver(),
		func(int64, []byte) (AppRegistrationVerifier, error) {
			t.Fatal("duplicate import attempted provider verification")
			return nil, nil
		})
	_, err := importer.Import(context.Background(), request)
	var importErr *AppRegistrationImportError
	if !errors.As(err, &importErr) || importErr.Code != AppRegistrationImportAlreadyRegistered ||
		importErr.ExistingRegistrationID != existing.ID {
		t.Fatalf("duplicate import error = %#v", err)
	}
}

func TestAppRegistrationImportRejectsIdentityAndPolicyMismatchWithoutSecrets(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*AuthenticatedApp)
		wantCode    AppRegistrationImportErrorCode
		wantProblem string
	}{
		{
			name:     "identity",
			mutate:   func(app *AuthenticatedApp) { app.OwnerLogin = "different-owner" },
			wantCode: AppRegistrationImportIdentityMismatch, wantProblem: "owner_login",
		},
		{
			name:     "policy",
			mutate:   func(app *AuthenticatedApp) { delete(app.Permissions, "contents") },
			wantCode: AppRegistrationImportPolicyMismatch, wantProblem: "permission:contents",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			repository := NewAppRegistrationRepository(store, newFakeConnectionSecrets())
			request := validAppRegistrationImportRequest(t)
			importer := NewAppRegistrationImporter(repository, importPublicResolver(),
				func(int64, []byte) (AppRegistrationVerifier, error) {
					return appRegistrationVerifierFunc(func(context.Context) (AuthenticatedApp, error) {
						app := authenticatedAppForImport(t, request)
						tt.mutate(&app)
						return app, nil
					}), nil
				})
			_, err := importer.Import(context.Background(), request)
			var importErr *AppRegistrationImportError
			if !errors.As(err, &importErr) || importErr.Code != tt.wantCode ||
				!containsString(importErr.Problems, tt.wantProblem) {
				t.Fatalf("import mismatch error = %#v", err)
			}
			for _, secret := range []string{request.ClientSecret, request.PrivateKey, request.WebhookSecret} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("import error leaked credential material")
				}
			}
			stored, getErr := store.GetAppRegistration(context.Background(), request.RegistrationID)
			if getErr != nil || stored != nil {
				t.Fatalf("mismatched registration persisted = %+v, err %v", stored, getErr)
			}
		})
	}
}

func TestAppRegistrationImportBoundsSecretInputsBeforeVerification(t *testing.T) {
	request := validAppRegistrationImportRequest(t)
	request.PrivateKey = strings.Repeat("k", maxAppRegistrationPrivateKey+1)
	importer := NewAppRegistrationImporter(
		NewAppRegistrationRepository(newTestStore(t), newFakeConnectionSecrets()),
		importPublicResolver(),
		func(int64, []byte) (AppRegistrationVerifier, error) {
			t.Fatal("oversized import reached verifier")
			return nil, nil
		},
	)
	_, err := importer.Import(context.Background(), request)
	if !IsAppRegistrationImportError(err, AppRegistrationImportInvalidRequest) {
		t.Fatalf("oversized import error = %#v", err)
	}
}

func TestAppRegistrationImportSanitizesVerificationFailure(t *testing.T) {
	request := validAppRegistrationImportRequest(t)
	importer := NewAppRegistrationImporter(
		NewAppRegistrationRepository(newTestStore(t), newFakeConnectionSecrets()),
		importPublicResolver(),
		func(int64, []byte) (AppRegistrationVerifier, error) {
			return nil, errors.New(request.ClientSecret + request.WebhookSecret + request.PrivateKey)
		},
	)
	_, err := importer.Import(context.Background(), request)
	if !IsAppRegistrationImportError(err, AppRegistrationImportVerificationFailed) {
		t.Fatalf("verification failure = %#v", err)
	}
	for _, secret := range []string{request.ClientSecret, request.WebhookSecret, request.PrivateKey} {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("verification error leaked imported credentials")
		}
	}
}

func TestAppRegistrationImportRejectsWrongExposedWebhookConfig(t *testing.T) {
	store := newTestStore(t)
	repository := NewAppRegistrationRepository(store, newFakeConnectionSecrets())
	request := validAppRegistrationImportRequest(t)
	importer := NewAppRegistrationImporter(repository, importPublicResolver(),
		func(int64, []byte) (AppRegistrationVerifier, error) {
			return appRegistrationVerifierWithWebhook{
				app: authenticatedAppForImport(t, request),
				config: AppWebhookConfig{
					URL: "https://wrong.example/webhook", ContentType: "json", InsecureSSL: "0",
				},
			}, nil
		})
	_, err := importer.Import(context.Background(), request)
	var importErr *AppRegistrationImportError
	if !errors.As(err, &importErr) || importErr.Code != AppRegistrationImportPolicyMismatch ||
		!containsString(importErr.Problems, "webhook") {
		t.Fatalf("webhook policy error = %#v", err)
	}
}

func TestAppRegistrationImportAcceptsDocumentedAuthenticatedAppShapeAndPersists(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	request := validAppRegistrationImportRequest(t)
	verified := false
	importer := NewAppRegistrationImporter(repository, importPublicResolver(),
		func(appID int64, privateKey []byte) (AppRegistrationVerifier, error) {
			if appID != request.AppID || string(privateKey) != request.PrivateKey {
				t.Fatalf("verifier credentials = app %d key %q", appID, privateKey)
			}
			return appRegistrationVerifierFunc(func(context.Context) (AuthenticatedApp, error) {
				verified = true
				return authenticatedAppForImport(t, request), nil
			}), nil
		})

	registration, err := importer.Import(context.Background(), request)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if !verified {
		t.Fatal("registration persisted without verifying GitHub App identity")
	}
	if registration == nil || registration.ID != request.RegistrationID ||
		registration.Source != AppRegistrationSourceImported || registration.AppID != request.AppID {
		t.Fatalf("imported registration = %+v", registration)
	}
	stored, credentials, err := repository.LoadRegistration(context.Background(), request.RegistrationID)
	if err != nil || stored == nil || stored.ID != request.RegistrationID {
		t.Fatalf("stored registration = %+v, err %v", stored, err)
	}
	if credentials.PrivateKey != request.PrivateKey || credentials.ClientSecret != request.ClientSecret ||
		credentials.WebhookSecret != request.WebhookSecret {
		t.Fatalf("stored credentials do not match import request")
	}
}

func validAppRegistrationImportRequest(t *testing.T) AppRegistrationImportRequest {
	t.Helper()
	_, privateKey := testAppPrivateKey(t)
	return AppRegistrationImportRequest{
		RegistrationID: "22222222-2222-4222-8222-222222222222",
		WorkspaceID:    "workspace-1", DisplayName: "Acme automation", GitHubHost: "github.com",
		AppID: 123, ClientID: "Iv1.client", ClientSecret: "client-secret",
		PrivateKey: string(privateKey), WebhookSecret: "webhook-secret", Slug: "kandev-acme",
		OwnerLogin: "acme", OwnerType: AppRegistrationOwnerOrganization,
		Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example",
	}
}

func authenticatedAppForImport(
	t *testing.T,
	request AppRegistrationImportRequest,
) AuthenticatedApp {
	t.Helper()
	submission, err := BuildAppRegistrationManifest(AppRegistrationManifestRequest{
		RegistrationID: request.RegistrationID, OwnerType: ManifestOwnerOrganization,
		OwnerLogin: request.OwnerLogin, PublicBaseURL: request.PublicBaseURL,
		Visibility: request.Visibility,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := submission.Manifest
	return AuthenticatedApp{
		ID: request.AppID, ClientID: request.ClientID, Slug: request.Slug,
		Name: request.DisplayName, OwnerLogin: request.OwnerLogin, OwnerType: string(request.OwnerType),
		ExternalURL: manifest.URL,
		Permissions: manifest.DefaultPermissions, Events: manifest.DefaultEvents,
	}
}

func importPublicResolver() PublicGitHubBaseURLResolver {
	return manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
	})
}
