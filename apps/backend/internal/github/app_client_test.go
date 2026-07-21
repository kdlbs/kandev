package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (f appRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testAppPrivateKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return key, pemBytes
}

func decodeJWTPart(t *testing.T, part string, out any) {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatalf("decode JWT part: %v", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal JWT part: %v", err)
	}
}

func TestAppClient_JWTClaimsAndSignature(t *testing.T) {
	key, pemBytes := testAppPrivateKey(t)
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	client, err := NewAppClient(12345, pemBytes)
	if err != nil {
		t.Fatalf("NewAppClient: %v", err)
	}
	client.now = func() time.Time { return now }

	token, err := client.AppJWT()
	if err != nil {
		t.Fatalf("AppJWT: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d, want 3", len(parts))
	}
	var header map[string]any
	decodeJWTPart(t, parts[0], &header)
	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		t.Fatalf("JWT header = %+v", header)
	}
	var claims map[string]any
	decodeJWTPart(t, parts[1], &claims)
	if claims["iss"] != "12345" {
		t.Fatalf("iss = %#v, want string app id", claims["iss"])
	}
	if got := int64(claims["iat"].(float64)); got != now.Add(-time.Minute).Unix() {
		t.Fatalf("iat = %d", got)
	}
	if got := int64(claims["exp"].(float64)); got != now.Add(9*time.Minute).Unix() {
		t.Fatalf("exp = %d", got)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("verify JWT: %v", err)
	}
}

func TestAppClient_MintInstallationTokenScopesPermissions(t *testing.T) {
	_, pemBytes := testAppPrivateKey(t)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	var requestBody map[string]any
	transport := appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/app/installations/77/access_tokens" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Fatalf("Authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		responseBody := `{"token":"ghs_install","expires_at":"` + expires.Format(time.RFC3339) + `","permissions":{"contents":"write","pull_requests":"write"}}`
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}, nil
	})

	client, err := NewAppClient(123, pemBytes)
	if err != nil {
		t.Fatalf("NewAppClient: %v", err)
	}
	client.baseURL = "https://api.test"
	client.httpClient.Transport = transport
	permissions := InstallationPermissions{"contents": PermissionWrite, "pull_requests": PermissionWrite}
	token, err := client.MintInstallationToken(context.Background(), 77, permissions, []string{"widgets"})
	if err != nil {
		t.Fatalf("MintInstallationToken: %v", err)
	}
	if token.Token != "ghs_install" || !token.ExpiresAt.Equal(expires) {
		t.Fatalf("token = %+v", token)
	}
	gotPermissions, ok := requestBody["permissions"].(map[string]any)
	if !ok || gotPermissions["contents"] != "write" || gotPermissions["pull_requests"] != "write" {
		t.Fatalf("request permissions = %#v", requestBody["permissions"])
	}
	if token.Principal.Kind != TokenCredentialInstallation || token.Principal.InstallationID != 77 {
		t.Fatalf("principal = %+v", token.Principal)
	}
	gotRepositories, ok := requestBody["repositories"].([]any)
	if !ok || len(gotRepositories) != 1 || gotRepositories[0] != "widgets" {
		t.Fatalf("request repositories = %#v", requestBody["repositories"])
	}
}

func TestAppClient_GetInstallation(t *testing.T) {
	_, pemBytes := testAppPrivateKey(t)
	transport := appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/app/installations/77" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"id":77,"account":{"id":9,"login":"acme","type":"Organization"},` +
					`"permissions":{"contents":"write"},"suspended_at":null}`,
			)),
		}, nil
	})
	client, err := NewAppClient(123, pemBytes)
	if err != nil {
		t.Fatalf("NewAppClient: %v", err)
	}
	client.baseURL = "https://api.test"
	client.httpClient.Transport = transport
	installation, err := client.GetInstallation(context.Background(), 77)
	if err != nil {
		t.Fatalf("GetInstallation: %v", err)
	}
	if installation.ID != 77 || installation.AccountID != 9 ||
		installation.AccountLogin != "acme" || installation.AccountType != "Organization" ||
		installation.Permissions["contents"] != PermissionWrite {
		t.Fatalf("installation = %+v", installation)
	}
}

func TestAppClient_GetAuthenticatedApp(t *testing.T) {
	_, pemBytes := testAppPrivateKey(t)
	transport := appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/app" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"id":123,"client_id":"Iv1.client","slug":"kandev-acme","name":"Kandev Acme",
				"owner":{"login":"acme","type":"Organization"},
				"external_url":"https://kandev.example",
				"permissions":{"contents":"write"},"events":["installation"]
			}`)),
		}, nil
	})
	client, err := NewAppClient(123, pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = "https://api.test"
	client.httpClient.Transport = transport
	app, err := client.GetAuthenticatedApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if app.ID != 123 || app.ClientID != "Iv1.client" || app.Slug != "kandev-acme" ||
		app.OwnerLogin != "acme" || app.OwnerType != "Organization" ||
		app.Permissions["contents"] != "write" || len(app.Events) != 1 {
		t.Fatalf("authenticated App = %+v", app)
	}
}

func TestAppClient_GetAuthenticatedAppBoundsProviderResponse(t *testing.T) {
	_, pemBytes := testAppPrivateKey(t)
	client, err := NewAppClient(123, pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = "https://api.test"
	client.httpClient.Transport = appRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				strings.Repeat("x", maxGitHubAppResponseSize+1),
			)),
		}, nil
	})
	_, err = client.GetAuthenticatedApp(context.Background())
	if !errors.Is(err, ErrGitHubAppResponseTooLarge) {
		t.Fatalf("oversized authenticated App error = %v, want %v", err, ErrGitHubAppResponseTooLarge)
	}
}

func TestAppClient_GetWebhookConfig(t *testing.T) {
	_, pemBytes := testAppPrivateKey(t)
	client, err := NewAppClient(123, pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = "https://api.test"
	client.httpClient.Transport = appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/app/hook/config" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"url":"https://kandev.example/webhook","content_type":"json","insecure_ssl":0}`,
			)),
		}, nil
	})
	config, err := client.GetWebhookConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.URL != "https://kandev.example/webhook" ||
		config.ContentType != "json" || config.InsecureSSL != "0" {
		t.Fatalf("webhook config = %+v", config)
	}
}

func TestAppClient_MapsInstallationPermissionsToCapabilities(t *testing.T) {
	permissions := InstallationPermissions{
		"metadata":       PermissionRead,
		"contents":       PermissionWrite,
		"pull_requests":  PermissionRead,
		"issues":         PermissionWrite,
		"checks":         PermissionRead,
		"administration": PermissionRead,
	}
	capabilities := CapabilitiesForPermissions(permissions)
	for _, capability := range []GitHubAppCapability{
		CapabilityRepositoryRead,
		CapabilityGitWrite,
		CapabilityPullRequestRead,
		CapabilityIssueWrite,
		CapabilityChecksRead,
		CapabilityBranchProtectionRead,
	} {
		if !capabilities[capability] {
			t.Errorf("capability %q = false", capability)
		}
	}
	if capabilities[CapabilityPullRequestWrite] {
		t.Error("read-only pull_requests unexpectedly grants write")
	}
}
