package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type manifestConversionRoundTripFunc func(*http.Request) (*http.Response, error)

func (f manifestConversionRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestManifestConversionParsesIdentityAndGeneratedCredentials(t *testing.T) {
	transport := manifestConversionRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost ||
			request.URL.String() != "https://api.test/app-manifests/code%2Fwith%2Fslashes/conversions" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Accept") != "application/vnd.github+json" ||
			request.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Fatalf("headers = %#v", request.Header)
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"id": 123,
				"node_id": "MDM6QXBwMTIz",
				"slug": "kandev-acme",
				"name": "Kandev Acme",
				"html_url": "https://github.com/apps/kandev-acme",
				"owner": {"id": 42, "login": "acme", "type": "Organization"},
				"client_id": "Iv1.client-id",
				"client_secret": "generated-client-secret",
				"webhook_secret": "generated-webhook-secret",
				"pem": "-----BEGIN RSA PRIVATE KEY-----\\nprivate-key-material\\n-----END RSA PRIVATE KEY-----",
				"permissions": {"contents": "write", "metadata": "read"},
				"events": ["installation", "installation_repositories", "github_app_authorization"]
			}`)),
		}, nil
	})
	client := NewManifestConversionClient()
	client.apiBaseURL = "https://api.test"
	client.httpClient.Transport = transport

	result, err := client.Convert(context.Background(), "code/with/slashes")
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if result.AppID != 123 || result.NodeID != "MDM6QXBwMTIz" || result.Slug != "kandev-acme" ||
		result.Name != "Kandev Acme" || result.Owner.ID != 42 || result.Owner.Login != "acme" ||
		result.Owner.Type != "Organization" || result.ClientID != "Iv1.client-id" ||
		result.ClientSecret != "generated-client-secret" ||
		result.WebhookSecret != "generated-webhook-secret" ||
		!strings.Contains(result.PrivateKeyPEM, "private-key-material") {
		t.Fatalf("Convert() = %+v", result.Redacted())
	}
	if result.Permissions["contents"] != "write" || len(result.Events) != 3 {
		t.Fatalf("manifest policy = %#v, %#v", result.Permissions, result.Events)
	}
	redacted := result.Redacted()
	if redacted.ClientSecret != "" || redacted.WebhookSecret != "" || redacted.PrivateKeyPEM != "" {
		t.Fatalf("Redacted() leaked credentials: %+v", redacted)
	}
}

func TestManifestConversionUsesStableSanitizedErrors(t *testing.T) {
	const (
		conversionCode = "one-time-conversion-code"
		clientSecret   = "generated-client-secret"
		webhookSecret  = "generated-webhook-secret"
		privateKey     = "generated-private-key"
	)
	tests := []struct {
		name       string
		transport  manifestConversionRoundTripFunc
		wantCode   ManifestConversionErrorCode
		wantStatus int
	}{
		{
			name: "transport failure",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed for " + conversionCode + " " + clientSecret)
			},
			wantCode: ManifestConversionRequestFailed,
		},
		{
			name: "rejected response",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnprocessableEntity,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"error":"` + clientSecret + ` ` + webhookSecret + ` ` + privateKey + `"}`,
					)),
				}, nil
			},
			wantCode:   ManifestConversionRejected,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "malformed response",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"client_secret":"` + clientSecret + `","pem":"` + privateKey,
					)),
				}, nil
			},
			wantCode: ManifestConversionInvalidResponse,
		},
		{
			name: "oversized response",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						strings.Repeat(privateKey, maxManifestConversionResponseSize),
					)),
				}, nil
			},
			wantCode: ManifestConversionResponseTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewManifestConversionClient()
			client.apiBaseURL = "https://api.test"
			client.httpClient.Transport = tt.transport
			_, err := client.Convert(context.Background(), conversionCode)
			var conversionErr *ManifestConversionError
			if !errors.As(err, &conversionErr) || conversionErr.Code != tt.wantCode ||
				conversionErr.StatusCode != tt.wantStatus {
				t.Fatalf("error = %#v, want code %q and status %d", err, tt.wantCode, tt.wantStatus)
			}
			message := err.Error()
			for _, secret := range []string{conversionCode, clientSecret, webhookSecret, privateKey} {
				if strings.Contains(message, secret) {
					t.Fatalf("error %q leaked %q", message, secret)
				}
			}
		})
	}
}

func TestManifestConversionRejectsMissingRequiredFieldsWithoutLeakingThem(t *testing.T) {
	const clientSecret = "secret-that-must-not-leak"
	transport := manifestConversionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"id":123,"client_secret":"` + clientSecret + `"}`,
			)),
		}, nil
	})
	client := NewManifestConversionClient()
	client.apiBaseURL = "https://api.test"
	client.httpClient.Transport = transport
	_, err := client.Convert(context.Background(), "code")
	var conversionErr *ManifestConversionError
	if !errors.As(err, &conversionErr) || conversionErr.Code != ManifestConversionInvalidResponse {
		t.Fatalf("error = %#v, want invalid response", err)
	}
	if strings.Contains(err.Error(), clientSecret) {
		t.Fatalf("error leaked client secret: %v", err)
	}
}
