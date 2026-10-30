package azuredevops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestEntraClient(t *testing.T, handler http.HandlerFunc) *entraHTTPClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newEntraHTTPClient()
	client.loginBaseURL = server.URL
	client.devAzureBaseURL = server.URL
	return client
}

func TestEntraDiscoverTenantReadsResourceTenantWithoutCredentials(t *testing.T) {
	const tenant = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	client := newTestEntraClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Path != "/acme/_apis/connectionData" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("tenant discovery must not send credentials")
		}
		w.Header().Add("X-VSS-ResourceTenant", "00000000-0000-0000-0000-000000000000, "+tenant)
		w.WriteHeader(http.StatusUnauthorized)
	})
	got, err := client.DiscoverTenant(context.Background(), "https://dev.azure.com/acme")
	if err != nil || got != tenant {
		t.Fatalf("tenant = %q, %v; want %q", got, err, tenant)
	}
}

func TestEntraDiscoverTenantFallsBackToOrganizations(t *testing.T) {
	for name, header := range map[string]string{
		"missing":   "",
		"zero GUID": "00000000-0000-0000-0000-000000000000",
		"malformed": "not-a-guid",
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestEntraClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if header != "" {
					w.Header().Set("X-VSS-ResourceTenant", header)
				}
				w.WriteHeader(http.StatusUnauthorized)
			})
			got, err := client.DiscoverTenant(context.Background(), "https://dev.azure.com/acme")
			if err != nil || got != defaultEntraTenant {
				t.Fatalf("tenant = %q, %v; want %q", got, err, defaultEntraTenant)
			}
		})
	}
}

func TestEntraDiscoverTenantRejectsInvalidOrganization(t *testing.T) {
	client := newEntraHTTPClient()
	if _, err := client.DiscoverTenant(context.Background(), "https://example.com/acme"); err == nil {
		t.Fatal("non-Azure organization URL must be rejected before any request")
	}
}

func TestEntraAuthorizeURLUsesPKCEAndAzureDevOpsScope(t *testing.T) {
	client := newEntraHTTPClient()
	raw := client.AuthorizeURL(entraAuthorizeRequest{
		Tenant: "tenant-1", ClientID: "client-1", RedirectURL: "http://localhost/cb",
		State: "state-1", CodeChallenge: "challenge-1",
	})
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "login.microsoftonline.com" || parsed.Path != "/tenant-1/oauth2/v2.0/authorize" {
		t.Fatalf("authorize endpoint = %s", raw)
	}
	query := parsed.Query()
	want := map[string]string{
		"client_id": "client-1", "response_type": "code", "redirect_uri": "http://localhost/cb",
		"response_mode": "query", "scope": entraScopes, "state": "state-1",
		"code_challenge": "challenge-1", "code_challenge_method": "S256", "prompt": "select_account",
	}
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	if !strings.Contains(entraScopes, "499b84ac-1321-427f-aa17-267ca6975798/.default") ||
		!strings.Contains(entraScopes, "offline_access") {
		t.Fatalf("scopes = %q, want Azure DevOps resource and offline access", entraScopes)
	}
}

func TestEntraExchangeCodeSendsPublicClientPKCEForm(t *testing.T) {
	client := newTestEntraClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tenant-1/oauth2/v2.0/token" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			"grant_type": "authorization_code", "client_id": "client-1", "code": "code-1",
			"redirect_uri": "http://localhost/cb", "code_verifier": "verifier-1", "scope": entraScopes,
		}
		for key, value := range want {
			if got := r.PostForm.Get(key); got != value {
				t.Errorf("%s = %q, want %q", key, got, value)
			}
		}
		if r.PostForm.Has("client_secret") {
			t.Error("public client must not send a client secret")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3599}`))
	})
	token, err := client.ExchangeCode(context.Background(), "tenant-1", "client-1", "code-1", "http://localhost/cb", "verifier-1")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access-1" || token.RefreshToken != "refresh-1" || token.ExpiresIn != 3599 {
		t.Fatalf("token = %+v", token)
	}
}

func TestEntraRefreshSendsRefreshGrant(t *testing.T) {
	client := newTestEntraClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("refresh_token") != "refresh-1" ||
			r.PostForm.Get("client_id") != "client-1" || r.PostForm.Get("scope") != entraScopes {
			t.Errorf("refresh form = %v", r.PostForm)
		}
		_, _ = w.Write([]byte(`{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600}`))
	})
	token, err := client.Refresh(context.Background(), "tenant-1", "client-1", "refresh-1")
	if err != nil || token.AccessToken != "access-2" || token.RefreshToken != "refresh-2" {
		t.Fatalf("token = %+v, %v", token, err)
	}
}

func TestEntraTokenErrorsExposeOnlyTheCode(t *testing.T) {
	client := newTestEntraClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"AADSTS70008: refresh-1 expired"}`))
	})
	_, err := client.Refresh(context.Background(), "tenant-1", "client-1", "refresh-1")
	var entraErr *EntraError
	if !errors.As(err, &entraErr) || entraErr.Code != "invalid_grant" || !entraErr.RequiresSignIn() {
		t.Fatalf("error = %#v, want invalid_grant requiring sign-in", err)
	}
	if strings.Contains(err.Error(), "refresh-1") {
		t.Fatalf("error leaks provider description: %q", err.Error())
	}
	transient := &EntraError{Code: "temporarily_unavailable", Status: http.StatusServiceUnavailable}
	if transient.RequiresSignIn() {
		t.Fatal("transient Entra errors must not require sign-in")
	}
}
