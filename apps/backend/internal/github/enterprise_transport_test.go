package github

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/githubhost"
)

type enterpriseTransportFunc func(*http.Request) (*http.Response, error)

func (f enterpriseTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2
func TestEnterpriseHostTransportEndpoints(t *testing.T) {
	for _, input := range []string{"github.com", "tenant.ghe.com", "git.example.com:8443"} {
		t.Run(input, func(t *testing.T) {
			h, err := githubhost.Parse(input)
			if err != nil {
				t.Fatal(err)
			}
			c := NewPATClientForHost("secret", h)
			var urls []string
			c.httpClient.Transport = enterpriseTransportFunc(func(r *http.Request) (*http.Response, error) {
				urls = append(urls, r.URL.String())
				if r.Header.Get("Authorization") != "token secret" || r.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
					t.Fatal("missing provider headers")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})
			var out map[string]any
			if err := c.get(context.Background(), "/user", &out); err != nil {
				t.Fatal(err)
			}
			if err := c.ExecuteGraphQL(context.Background(), "query { viewer { login } }", nil, &out); err != nil {
				t.Fatal(err)
			}
			if len(urls) != 2 || urls[0] != h.RESTURL()+"/user" || urls[1] != h.GraphQLURL() {
				t.Fatalf("wrong destination: %v", urls)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.3
func TestEnterpriseHostTransportPaginationBoundary(t *testing.T) {
	c := NewPATClient("secret")
	requests := 0
	c.httpClient.Transport = enterpriseTransportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	for _, endpoint := range []string{"https://attacker.example/user", "//attacker.example/user", "/../user", "/%2e%2e/user", "/%2fuser", "https://api.github.com.attacker.example/user", "https://user@api.github.com/user"} {
		var out map[string]any
		if err := c.get(context.Background(), endpoint, &out); err == nil {
			t.Errorf("accepted unsafe endpoint %q", endpoint)
		}
	}
	if requests != 0 {
		t.Fatalf("sent %d unsafe authenticated requests", requests)
	}
}
func TestEnterpriseHostTransportClientFamilies(t *testing.T) {
	h, err := githubhost.Parse("git.example.com")
	if err != nil {
		t.Fatal(err)
	}
	oauth := NewGitHubOAuthClientForHost("id", "secret", h)
	manifest := NewManifestConversionClientForHost(h)
	if oauth.webBaseURL != h.WebURL() || oauth.apiBaseURL != h.RESTURL() || manifest.apiBaseURL != h.RESTURL() {
		t.Fatalf("provider lifecycle clients ignored host: %s %s %s", oauth.webBaseURL, oauth.apiBaseURL, manifest.apiBaseURL)
	}
}

func TestEnterpriseHostTransportAppEndpoint(t *testing.T) {
	_, key := testAppPrivateKey(t)
	h, err := githubhost.Parse("git.example.com")
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewAppClientForHost(123, key, h)
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != h.RESTURL() {
		t.Fatalf("App destination %s", c.baseURL)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.4
func TestEnterpriseHostTransportTrustedTLSAndRedirect(t *testing.T) {
	hits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/api/v3/redirect" {
			w.Header().Set("Location", "https://attacker.example/user")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	h, err := githubhost.Parse("git.private.example")
	if err != nil {
		t.Fatal(err)
	}
	c := NewPATClientForHost("secret", h)
	trusted := srv.Client()
	c.httpClient.Transport = enterpriseTransportFunc(func(r *http.Request) (*http.Response, error) {
		cloned := r.Clone(r.Context())
		target, parseErr := url.Parse(srv.URL)
		if parseErr != nil {
			return nil, parseErr
		}
		cloned.URL.Scheme = target.Scheme
		cloned.URL.Host = target.Host
		return trusted.Transport.RoundTrip(cloned)
	})
	var out map[string]any
	if err := c.get(context.Background(), "/user", &out); err != nil {
		t.Fatal(err)
	}
	if err := c.get(context.Background(), "/redirect", &out); err == nil {
		t.Fatal("followed authenticated redirect")
	}
	if hits != 2 {
		t.Fatalf("redirect reached fixture %d times", hits)
	}
}

func TestEnterpriseHostTransportDialDestinations(t *testing.T) {
	for _, input := range []string{"127.0.0.1", "::1", "169.254.169.254", "fe80::1", "0.0.0.0", "::", "224.0.0.1", "ff02::1", "::ffff:127.0.0.1"} {
		if !forbiddenGitHubIP(net.ParseIP(input)) {
			t.Errorf("allowed forbidden direct destination %s", input)
		}
	}
	for _, input := range []string{"10.0.0.1", "192.168.1.1", "172.16.0.1", "fd00::1", "140.82.112.5"} {
		if forbiddenGitHubIP(net.ParseIP(input)) {
			t.Errorf("blocked trusted private/public destination %s", input)
		}
	}
}

func TestEnterpriseHostTransportTrustedOperatorProxy(t *testing.T) {
	hits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("{}"))
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = original }()
	client := newGitHubHTTPClient(time.Second)
	response, err := client.Get("http://git.private.example/user")
	if err != nil {
		t.Fatalf("trusted operator proxy blocked: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if hits != 1 {
		t.Fatalf("proxy received %d requests", hits)
	}
}

func TestEnterpriseHostTransportInjectedDefault(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = enterpriseTransportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"fixture"}`))}, nil
	})
	defer func() { http.DefaultTransport = original }()
	client := NewPATClient("secret")
	login, err := client.GetAuthenticatedUser(context.Background())
	if err != nil || login != "fixture" {
		t.Fatalf("injected transport failed: %s %v", login, err)
	}
}
