package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateHost(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantErr  string // substring; "" = success
		wantPath string // expected base.Path after validation
	}{
		{"default-when-empty", "", "", ""},
		{"https-bare", "https://gitlab.example.com", "", ""},
		{"http-allowed", "http://gitlab.local", "", ""},
		{"trailing-slash-trimmed", "https://gitlab.example.com/", "", ""},
		{"subpath-preserved", "https://gitlab.acme.corp/gitlab", "", "/gitlab"},
		{"subpath-trailing-slash-trimmed", "https://gitlab.acme.corp/gitlab/", "", "/gitlab"},
		{"missing-scheme", "gitlab.example.com", "scheme", ""},
		{"ftp-scheme-rejected", "ftp://gitlab.example.com", "scheme", ""},
		{"empty-host", "https://", "hostname", ""},
		{"userinfo-rejected", "https://user:pw@gitlab.example.com", "userinfo", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateHost(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tc.wantPath)
			}
			if got.Scheme != "http" && got.Scheme != "https" {
				t.Errorf("scheme escaped validation: %q", got.Scheme)
			}
		})
	}
}

func TestCheckHost_ProbesVersionEndpoint(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/v4/version" {
			t.Errorf("path = %q, want /api/v4/version", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized) // 401 still counts as reachable
	}))
	defer srv.Close()

	if err := CheckHost(context.Background(), srv.URL); err != nil {
		t.Fatalf("err = %v, want nil (401 is a reachable signal)", err)
	}
	if !called {
		t.Fatal("server was not called")
	}
}

func TestCheckHost_PreservesBasePath(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/gitlab/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"version":"16.0.0"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if err := CheckHost(context.Background(), srv.URL+"/gitlab"); err != nil {
		t.Fatalf("err = %v, want nil for host with /gitlab base path", err)
	}
	if !called {
		t.Fatal("server was not called at /gitlab/api/v4/version — base path was dropped")
	}
}

func TestCheckHost_RejectsBadScheme(t *testing.T) {
	if err := CheckHost(context.Background(), "ftp://gitlab.example.com"); err == nil {
		t.Fatal("expected scheme rejection")
	}
}

func TestCheckHost_UnreachableHostReturnsError(t *testing.T) {
	// 192.0.2.0/24 is reserved for documentation (RFC 5737); connecting
	// to it deterministically fails.
	err := CheckHost(context.Background(), "http://192.0.2.1:1")
	if err == nil {
		t.Fatal("expected network error")
	}
}
