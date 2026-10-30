package azuredevops

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// rotatingAuthorizer issues bearer-1 until renewed, then bearer-2.
type rotatingAuthorizer struct {
	mu       sync.Mutex
	token    string
	renewals []string
	canRenew bool
	renewErr error
}

func (a *rotatingAuthorizer) Header(context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return bearerPrefix + a.token, nil
}

func (a *rotatingAuthorizer) Renew(_ context.Context, rejected string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.renewals = append(a.renewals, rejected)
	if a.renewErr != nil {
		return false, a.renewErr
	}
	if !a.canRenew {
		return false, nil
	}
	a.token = "bearer-2"
	return true, nil
}

func (a *rotatingAuthorizer) Redact(text string) string { return text }

func newAuthorizedTestClient(t *testing.T, auth Authorizer, handler http.HandlerFunc) *RESTClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		server.Config.Handler.ServeHTTP(response, req)
		return response.Result(), nil
	})}
	return NewRESTClient("https://dev.azure.com/acme", auth, httpClient)
}

func TestRESTClientRetriesOnceWithRenewedBearerAndReplaysBody(t *testing.T) {
	auth := &rotatingAuthorizer{token: "bearer-1", canRenew: true}
	var mu sync.Mutex
	var seen []string
	var bodies []string
	client := newAuthorizedTestClient(t, auth, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer bearer-2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"workItems":[]}`))
	})
	if _, err := client.QueryWIQL(context.Background(), "project-1", "SELECT [System.Id] FROM WorkItems", 5); err != nil {
		t.Fatalf("QueryWIQL: %v", err)
	}
	if len(seen) != 2 || seen[0] != "Bearer bearer-1" || seen[1] != "Bearer bearer-2" {
		t.Fatalf("authorization headers = %v", seen)
	}
	if bodies[0] == "" || bodies[0] != bodies[1] {
		t.Fatalf("retried body = %q, want the original %q", bodies[1], bodies[0])
	}
	if len(auth.renewals) != 1 || auth.renewals[0] != "Bearer bearer-1" {
		t.Fatalf("renewals = %v", auth.renewals)
	}
}

func TestRESTClientDoesNotRetryTwiceOrWithoutRenewal(t *testing.T) {
	for name, auth := range map[string]*rotatingAuthorizer{
		"renewed but still rejected": {token: "bearer-1", canRenew: true},
		"not renewable":              {token: "bearer-1"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls int
			client := newAuthorizedTestClient(t, auth, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(http.StatusUnauthorized)
			})
			_, err := client.ListProjects(context.Background())
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
				t.Fatalf("error = %v, want 401 APIError", err)
			}
			wantCalls := 1
			if auth.canRenew {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("requests = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestRESTClientSurfacesRenewalFailure(t *testing.T) {
	auth := &rotatingAuthorizer{token: "bearer-1", renewErr: ErrReauthRequired}
	client := newAuthorizedTestClient(t, auth, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := client.ListProjects(context.Background())
	if !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("error = %v, want ErrReauthRequired", err)
	}
}

func TestRESTClientRedactsBearerFromErrors(t *testing.T) {
	auth := staticBearerAuthorizer{token: "secret-bearer"}
	client := newAuthorizedTestClient(t, auth, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad token secret-bearer"}`))
	})
	_, err := client.ListProjects(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret-bearer") {
		t.Fatalf("error = %v, want redacted bearer", err)
	}
}
