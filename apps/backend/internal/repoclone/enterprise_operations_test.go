package repoclone

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.3
func TestEnterpriseRealGitCloneFetchPush(t *testing.T) {
	fixture, _, _, allowed := checkoutOptionsGitServer(t)
	endpoint, _ := url.Parse(fixture)
	proxy := enterpriseGitFixtureProxy(t, endpoint.Host)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	t.Setenv("GIT_SSL_NO_VERIFY", "true") // Only the disposable TLS fixture certificate.
	for _, host := range []string{"github.com", "acme.ghe.com", "git.company.example"} {
		t.Run(host, func(t *testing.T) {
			allowed.Store(true)
			cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
			cloner.SetGitCredentialProvider(&recordingCredentialProvider{password: "fixture-token"})
			origin := "https://" + host
			request := GitCredentialRequest{WorkspaceID: "workspace", Provider: "github", ProviderHost: origin, CloneURL: origin + "/repo.git", Owner: "acme", Name: "repo"}
			path, err := cloner.EnsureWorkspaceClonedWithCredentialRequest(context.Background(), request, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := cloner.RefreshWorkspaceRepositoryWithCredentialRequest(context.Background(), request, path, "", ""); err != nil {
				t.Fatal(err)
			}
			run := func(args ...string) ([]byte, error) {
				return cloner.runConfiguredGitCombined(context.Background(), gitFetchTimeout, append([]string{"-C", path}, args...), func(cmd *exec.Cmd) (func(), error) {
					return configureGitCommand(cmd, &cloneAuth{origin: origin, username: "x-access-token", password: "fixture-token"})
				})
			}
			if output, err := run("push", "origin", "HEAD:refs/heads/enterprise-fixture-"+strings.ReplaceAll(host, ".", "-")); err != nil {
				t.Fatalf("push: %s %v", output, err)
			}
			if remote := strings.TrimSpace(checkoutOptionsGit(t, path, "remote", "get-url", "origin")); remote != request.CloneURL {
				t.Fatalf("stored clone origin = %q", remote)
			}
			allowed.Store(false)
			if _, err := run("fetch", "origin"); err == nil {
				t.Fatal("fetch succeeded after token revocation")
			}
		})
	}
}

func enterpriseGitFixtureProxy(t *testing.T, target string) *httptest.Server {
	t.Helper()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		upstream, err := net.Dial("tcp", target)
		if err != nil {
			http.Error(w, "fixture unavailable", http.StatusBadGateway)
			return
		}
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	return proxy
}
