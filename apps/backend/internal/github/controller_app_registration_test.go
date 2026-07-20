package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/config"
)

func TestHTTPDeploymentAppRegistrationStartCallbackAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	conversion := testManifestConversion(t, "acme", "Organization")
	service.deploymentAppRegistration = NewDeploymentAppRegistrationService(
		DeploymentAppRegistrationConfig{
			Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
			Converter: manifestConverterFunc(func(context.Context, string) (ManifestConversionResult, error) {
				return conversion, nil
			}),
			Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
			}),
			Random: strings.NewReader(strings.Repeat("h", oauthRandomBytes)),
		},
	)
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

	start := httptest.NewRecorder()
	startRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/github/app/registration/start",
		strings.NewReader(`{"owner_type":"organization","owner_login":"acme","public_base_url":"https://kandev.example"}`),
	)
	startRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(start, startRequest)
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", start.Code, start.Body.String())
	}
	var started DeploymentAppRegistrationStart
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil || started.State == "" {
		t.Fatalf("start response = %s, error = %v", start.Body.String(), err)
	}
	for name, path := range map[string]string{
		"malformed state": "/api/v1/github/app/registration/callback?state=not-base64&code=ignored",
		"short state": "/api/v1/github/app/registration/callback?state=" +
			base64.RawURLEncoding.EncodeToString(make([]byte, oauthRandomBytes-1)) + "&code=ignored",
		"oversized state": "/api/v1/github/app/registration/callback?state=" +
			strings.Repeat("a", 1025) + "&code=ignored",
		"oversized code": "/api/v1/github/app/registration/callback?state=" + started.State +
			"&code=" + strings.Repeat("c", 1025),
		"oversized error": "/api/v1/github/app/registration/callback?state=" + started.State +
			"&error=" + strings.Repeat("e", 257),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assertPrivateDeploymentAppRedirect(
				t,
				response,
				"/settings/system/github-app?github_app_result=github_app_invalid_callback",
			)
		})
	}

	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/app/registration/callback?state="+started.State+"&code=one-time-code",
		nil,
	))
	assertPrivateDeploymentAppRedirect(
		t, callback, "/settings/system/github-app?github_app_result=connected",
	)
	for _, secret := range []string{conversion.ClientSecret, conversion.WebhookSecret, conversion.PrivateKeyPEM} {
		if strings.Contains(callback.Body.String()+callback.Header().Get("Location"), secret) {
			t.Fatalf("callback response leaked a generated credential")
		}
	}

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/github/app/registration", nil))
	if status.Code != http.StatusOK || strings.Contains(status.Body.String(), conversion.ClientSecret) ||
		!strings.Contains(status.Body.String(), `"source":"managed"`) ||
		!strings.Contains(status.Body.String(), `"ready":true`) {
		t.Fatalf("status = %d, body = %s", status.Code, status.Body.String())
	}
}

func TestHTTPDeploymentAppRegistrationCancellationConsumesActiveAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	service.deploymentAppRegistration = NewDeploymentAppRegistrationService(
		DeploymentAppRegistrationConfig{
			Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
			Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
			}),
			Random: strings.NewReader(strings.Repeat("x", oauthRandomBytes)),
		},
	)
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

	start := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/github/app/registration/start",
		strings.NewReader(`{"owner_type":"organization","owner_login":"acme","public_base_url":"https://kandev.example"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(start, request)
	var started DeploymentAppRegistrationStart
	if start.Code != http.StatusOK || json.Unmarshal(start.Body.Bytes(), &started) != nil {
		t.Fatalf("start = %d, body = %s", start.Code, start.Body.String())
	}

	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/app/registration/callback?state="+started.State+"&error=access_denied",
		nil,
	))
	assertPrivateDeploymentAppRedirect(
		t,
		callback,
		"/settings/system/github-app?github_app_result=github_app_registration_cancelled",
	)
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/github/app/registration", nil))
	if status.Code != http.StatusOK || strings.Contains(status.Body.String(), `"state":"registering"`) {
		t.Fatalf("status after cancellation = %d, body = %s", status.Code, status.Body.String())
	}
}

func assertPrivateDeploymentAppRedirect(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantLocation string,
) {
	t.Helper()
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != wantLocation {
		t.Fatalf(
			"callback = %d %q, body = %s",
			response.Code,
			response.Header().Get("Location"),
			response.Body,
		)
	}
	if response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback privacy headers = %v", response.Header())
	}
	location := response.Header().Get("Location")
	if strings.Contains(location, "code=") || strings.Contains(location, "state=") {
		t.Fatalf("callback redirect leaked provider parameters: %q", location)
	}
}

func TestHTTPDeploymentAppEnvironmentRegistrationIsReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	service.deploymentAppRegistration = NewDeploymentAppRegistrationService(
		DeploymentAppRegistrationConfig{
			Environment: testEnvironmentDeploymentAppConfig(t),
			Repository:  NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
		},
	)
	if err := service.deploymentAppRegistration.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/github/app/registration", nil))
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), `"code":"github_app_environment_read_only"`) {
		t.Fatalf("delete = %d, body = %s", response.Code, response.Body.String())
	}
	for _, secret := range []string{
		service.deploymentAppRegistration.environment.ClientSecret,
		service.deploymentAppRegistration.environment.WebhookSecret,
		service.deploymentAppRegistration.environment.PrivateKey,
	} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("read-only error leaked environment credentials")
		}
	}
}

func TestHTTPDeploymentAppPartialEnvironmentStatusIsSanitized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	const sensitive = "do-not-return-this-client-secret"
	const urlSensitive = "do-not-return-this-url-marker"
	service.deploymentAppRegistration = NewDeploymentAppRegistrationService(
		DeploymentAppRegistrationConfig{
			Environment: config.GitHubAppConfig{
				ClientSecret:  sensitive,
				PublicBaseURL: "https://operator:password@example.com/?token=" + urlSensitive,
			},
			Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
		},
	)
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/github/app/registration", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), sensitive) ||
		strings.Contains(response.Body.String(), "operator") ||
		strings.Contains(response.Body.String(), urlSensitive) ||
		strings.Contains(response.Body.String(), `"registration"`) ||
		!strings.Contains(response.Body.String(), `"unavailable_code":"github_app_environment_invalid"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMockDeploymentAppRegistrationStatesAndReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	mock := NewMockClient()
	service := NewService(mock, "mock", nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	service.SetDeploymentAppRegistrationService(NewDeploymentAppRegistrationService(
		DeploymentAppRegistrationConfig{
			Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
		},
	))
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)
	NewMockController(mock, nil, nil, service, testLogger(t)).RegisterRoutes(router)

	seed := httptest.NewRecorder()
	seedRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/github/mock/deployment-app-registration",
		strings.NewReader(`{
			"source":"managed","state":"invalid","ready":false,
			"app_id":123,"slug":"kandev-acme","owner_login":"acme",
			"owner_type":"Organization","public_base_url":"https://kandev.example",
			"webhook_status":"failing","unavailable_code":"github_app_managed_invalid"
		}`),
	)
	seedRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(seed, seedRequest)
	if seed.Code != http.StatusOK {
		t.Fatalf("seed = %d, body = %s", seed.Code, seed.Body.String())
	}
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/github/app/registration", nil))
	if status.Code != http.StatusOK ||
		!strings.Contains(status.Body.String(), `"state":"invalid"`) ||
		!strings.Contains(status.Body.String(), `"webhook_status":"failing"`) {
		t.Fatalf("mock status = %d, body = %s", status.Code, status.Body.String())
	}

	reset := httptest.NewRecorder()
	router.ServeHTTP(reset, httptest.NewRequest(http.MethodDelete, "/api/v1/github/mock/reset", nil))
	if reset.Code != http.StatusOK {
		t.Fatalf("reset = %d, body = %s", reset.Code, reset.Body.String())
	}
	status = httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/github/app/registration", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"state":"unconfigured"`) {
		t.Fatalf("status after reset = %d, body = %s", status.Code, status.Body.String())
	}
}
