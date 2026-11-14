package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const installationAccessTestToken = "user-access-token"

type installationListEntry struct {
	ID int64 `json:"id"`
}

type installationListPayload struct {
	TotalCount    int                     `json:"total_count"`
	Installations []installationListEntry `json:"installations"`
}

func newInstallationAccessTestClient(server *httptest.Server) *GitHubOAuthClient {
	client := NewGitHubOAuthClient("client-id", "client-secret")
	client.apiBaseURL = server.URL
	client.httpClient = server.Client()
	return client
}

func writeInstallationListPayload(w http.ResponseWriter, totalCount int, ids ...int64) {
	payload := installationListPayload{TotalCount: totalCount}
	for _, id := range ids {
		payload.Installations = append(payload.Installations, installationListEntry{ID: id})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func installationPageHandler(pages map[int][]int64, totalCount int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil {
			http.Error(w, "invalid page", http.StatusBadRequest)
			return
		}
		writeInstallationListPayload(w, totalCount, pages[page]...)
	}
}

func TestOAuthInstallationAccessPagination(t *testing.T) {
	tests := []struct {
		name           string
		pages          map[int][]int64
		totalCount     int
		installationID int64
		wantAccessible bool
		wantPages      []string
	}{
		{
			name: "first page match among unrelated ids", pages: map[int][]int64{1: {7, 42, 9}},
			totalCount: 3, installationID: 42, wantAccessible: true, wantPages: []string{"1"},
		},
		{
			name: "later page match", pages: map[int][]int64{1: {7, 8}, 2: {42}},
			totalCount: 3, installationID: 42, wantAccessible: true, wantPages: []string{"1", "2"},
		},
		{
			name: "unrelated ids exhaust total count", pages: map[int][]int64{1: {7, 8}},
			totalCount: 2, installationID: 42, wantAccessible: false, wantPages: []string{"1"},
		},
		{
			name: "empty inventory", totalCount: 0, installationID: 42,
			wantAccessible: false, wantPages: []string{"1"},
		},
		{
			name: "empty later page stops search", pages: map[int][]int64{1: {7, 8}},
			totalCount: 250, installationID: 42, wantAccessible: false, wantPages: []string{"1", "2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var requested []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requested = append(requested, r.URL.Query().Get("page"))
				mu.Unlock()
				installationPageHandler(test.pages, test.totalCount)(w, r)
			}))
			defer server.Close()

			accessible, err := newInstallationAccessTestClient(server).UserCanAccessInstallation(
				context.Background(), installationAccessTestToken, test.installationID,
			)
			if err != nil || accessible != test.wantAccessible {
				t.Fatalf("UserCanAccessInstallation() = %v, %v; want %v", accessible, err, test.wantAccessible)
			}
			mu.Lock()
			defer mu.Unlock()
			if got, want := strings.Join(requested, ","), strings.Join(test.wantPages, ","); got != want {
				t.Fatalf("requested pages = %q, want %q", got, want)
			}
		})
	}
}

func TestOAuthInstallationAccessRequestContract(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		wantPath string
	}{
		{name: "public host", wantPath: "/user/installations"},
		{name: "host REST prefix", basePath: "/api/v3", wantPath: "/api/v3/user/installations"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if err := validateInstallationAccessRequest(r, test.wantPath); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeInstallationListPayload(w, 1, 42)
			}))
			defer server.Close()

			client := newInstallationAccessTestClient(server)
			client.apiBaseURL = server.URL + test.basePath
			accessible, err := client.UserCanAccessInstallation(
				context.Background(), installationAccessTestToken, 42,
			)
			if err != nil || !accessible {
				t.Fatalf("UserCanAccessInstallation() = %v, %v; want true, nil", accessible, err)
			}
			if requests.Load() != 1 {
				t.Fatalf("installation list requests = %d, want 1", requests.Load())
			}
		})
	}
}

func validateInstallationAccessRequest(r *http.Request, wantPath string) error {
	if r.Method != http.MethodGet {
		return errors.New("installation list must use GET")
	}
	if r.URL.Path != wantPath {
		return errors.New("unexpected installation list path: " + r.URL.Path)
	}
	query := r.URL.Query()
	if query.Get("per_page") != "100" || query.Get("page") != "1" {
		return errors.New("unexpected installation list pagination")
	}
	if r.Header.Get("Authorization") != "Bearer "+installationAccessTestToken {
		return errors.New("installation list missing bearer token")
	}
	if r.Header.Get("Accept") != githubAccept || r.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
		return errors.New("installation list missing GitHub API headers")
	}
	return nil
}

func TestOAuthInstallationAccessInvalidID(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeInstallationListPayload(w, 0)
	}))
	defer server.Close()
	client := newInstallationAccessTestClient(server)

	for _, installationID := range []int64{0, -1, -42} {
		accessible, err := client.UserCanAccessInstallation(
			context.Background(), installationAccessTestToken, installationID,
		)
		if err != nil || accessible {
			t.Fatalf("UserCanAccessInstallation(%d) = %v, %v; want false, nil", installationID, accessible, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("installation list requests = %d, want 0 for non-positive ids", requests.Load())
	}
}

func TestOAuthInstallationAccessErrors(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusCreated,
	} {
		t.Run("status "+strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			accessible, err := newInstallationAccessTestClient(server).UserCanAccessInstallation(
				context.Background(), installationAccessTestToken, 42,
			)
			assertGitHubAPIError(t, err, status)
			if accessible {
				t.Fatal("accessible = true on provider error")
			}
		})
	}

	t.Run("invalid json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"total_count":`)
		}))
		defer server.Close()

		accessible, err := newInstallationAccessTestClient(server).UserCanAccessInstallation(
			context.Background(), installationAccessTestToken, 42,
		)
		var apiErr *GitHubAPIError
		if err == nil || errors.As(err, &apiErr) || accessible {
			t.Fatalf("UserCanAccessInstallation() = %v, %v; want decoding error", accessible, err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		client := newInstallationAccessTestClient(server)
		server.Close()

		accessible, err := client.UserCanAccessInstallation(
			context.Background(), installationAccessTestToken, 42,
		)
		if err == nil || accessible {
			t.Fatalf("UserCanAccessInstallation() = %v, %v; want transport error", accessible, err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		accessible, err := newInstallationAccessTestClient(server).UserCanAccessInstallation(
			ctx, installationAccessTestToken, 42,
		)
		if err == nil || accessible {
			t.Fatalf("UserCanAccessInstallation() = %v, %v; want cancellation error", accessible, err)
		}
	})

	t.Run("error on later page", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.URL.Query().Get("page") == "1" {
				writeInstallationListPayload(w, 2, 7)
				return
			}
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		accessible, err := newInstallationAccessTestClient(server).UserCanAccessInstallation(
			context.Background(), installationAccessTestToken, 42,
		)
		assertGitHubAPIError(t, err, http.StatusBadGateway)
		if accessible || requests.Load() != 2 {
			t.Fatalf("accessible = %v, requests = %d; want false, 2", accessible, requests.Load())
		}
	})
}

func assertGitHubAPIError(t *testing.T, err error, status int) {
	t.Helper()
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
		t.Fatalf("error = %v, want GitHubAPIError with status %d", err, status)
	}
}

type installationAccessTrackedBody struct {
	io.ReadCloser
	closed atomic.Bool
}

func (b *installationAccessTrackedBody) Close() error {
	b.closed.Store(true)
	return b.ReadCloser.Close()
}

type installationAccessTrackingTransport struct {
	base   http.RoundTripper
	mu     sync.Mutex
	bodies []*installationAccessTrackedBody
}

func (t *installationAccessTrackingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body := &installationAccessTrackedBody{ReadCloser: response.Body}
	response.Body = body
	t.mu.Lock()
	t.bodies = append(t.bodies, body)
	t.mu.Unlock()
	return response, nil
}

func (t *installationAccessTrackingTransport) trackedBodies() []*installationAccessTrackedBody {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*installationAccessTrackedBody(nil), t.bodies...)
}

func TestOAuthInstallationAccessClosesPageBodies(t *testing.T) {
	t.Run("each page before the next request", func(t *testing.T) {
		transport := &installationAccessTrackingTransport{}
		pages := map[int][]int64{1: {7}, 2: {8}, 3: {42}}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") != "1" {
				assertTrackedInstallationBodiesClosed(t, transport)
			}
			installationPageHandler(pages, 3)(w, r)
		}))
		defer server.Close()
		transport.base = server.Client().Transport
		client := newInstallationAccessTestClient(server)
		client.httpClient = &http.Client{Transport: transport}

		accessible, err := client.UserCanAccessInstallation(
			context.Background(), installationAccessTestToken, 42,
		)
		if err != nil || !accessible {
			t.Fatalf("UserCanAccessInstallation() = %v, %v; want true, nil", accessible, err)
		}
		if bodies := transport.trackedBodies(); len(bodies) != 3 {
			t.Fatalf("tracked bodies = %d, want 3", len(bodies))
		}
		assertTrackedInstallationBodiesClosed(t, transport)
	})

	t.Run("error response body", func(t *testing.T) {
		transport := &installationAccessTrackingTransport{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"total_count":`)
		}))
		defer server.Close()
		transport.base = server.Client().Transport
		client := newInstallationAccessTestClient(server)
		client.httpClient = &http.Client{Transport: transport}

		if _, err := client.UserCanAccessInstallation(
			context.Background(), installationAccessTestToken, 42,
		); err == nil {
			t.Fatal("UserCanAccessInstallation() error = nil, want decoding error")
		}
		if bodies := transport.trackedBodies(); len(bodies) != 1 {
			t.Fatalf("tracked bodies = %d, want 1", len(bodies))
		}
		assertTrackedInstallationBodiesClosed(t, transport)
	})
}

func assertTrackedInstallationBodiesClosed(t *testing.T, transport *installationAccessTrackingTransport) {
	t.Helper()
	for index, body := range transport.trackedBodies() {
		if !body.closed.Load() {
			t.Errorf("installation response body %d was not closed", index)
		}
	}
}
