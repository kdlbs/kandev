package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

func newHostAPITarget(t *testing.T, wantToken string, seen *[]string) *httptest.Server {
	t.Helper()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.Path)
		switch {
		case r.Header.Get(agentctltypes.HostAPIProbeTokenHeader) == "redirect-token":
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		case r.Header.Get(agentctltypes.HostAPIProbeTokenHeader) == wantToken:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(target.Close)
	return target
}

func TestProbeHostAPIRequestsThePingRouteWithTheToken(t *testing.T) {
	var seen []string
	target := newHostAPITarget(t, "probe-token", &seen)
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")

	if err := client.ProbeHostAPI(context.Background(), target.URL, "probe-token"); err != nil {
		t.Fatalf("ProbeHostAPI(): %v", err)
	}
	if len(seen) != 1 || seen[0] != "GET "+agentctltypes.HostAPIPingPath {
		t.Fatalf("target saw %v, want exactly one GET of the ping route", seen)
	}
}

func TestProbeHostAPIReportsAnyReplyThatIsNot204(t *testing.T) {
	var seen []string
	target := newHostAPITarget(t, "probe-token", &seen)
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")

	err := client.ProbeHostAPI(context.Background(), target.URL, "wrong-token")
	var rejected *agentctl.HostAPIProbeError
	if !errors.As(err, &rejected) || rejected.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ProbeHostAPI() = %v, want a probe error with status 401", err)
	}
}

func TestProbeHostAPIDoesNotFollowRedirects(t *testing.T) {
	var seen []string
	target := newHostAPITarget(t, "probe-token", &seen)
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")

	err := client.ProbeHostAPI(context.Background(), target.URL, "redirect-token")
	var rejected *agentctl.HostAPIProbeError
	if !errors.As(err, &rejected) || rejected.StatusCode != http.StatusFound {
		t.Fatalf("ProbeHostAPI() = %v, want a probe error with status 302", err)
	}
	for _, line := range seen {
		if line == "GET /elsewhere" {
			t.Fatal("the probe followed a redirect")
		}
	}
}

func TestProbeHostAPIRefusesAnyURLThatIsNotThePingRoute(t *testing.T) {
	var seen []string
	target := newHostAPITarget(t, "probe-token", &seen)
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")

	for name, raw := range map[string]string{
		"other path":  target.URL + "/api/v1/tasks",
		"query":       target.URL + agentctltypes.HostAPIPingPath + "?x=1",
		"userinfo":    (&url.URL{Scheme: "http", User: url.UserPassword("probe-user", "probe-pass"), Host: target.Listener.Addr().String(), Path: agentctltypes.HostAPIPingPath}).String(),
		"file scheme": "file:///etc/passwd",
		"no host":     "http:///api/v1/runtime/ping",
	} {
		err := client.ProbeHostAPIURL(context.Background(), raw, "probe-token")
		if err == nil {
			t.Fatalf("%s: probe of %q succeeded", name, raw)
		}
	}
	if len(seen) != 0 {
		t.Fatalf("a refused probe still reached the target: %v", seen)
	}
}

func TestProbeHostAPIUnreachableTargetIsAnError(t *testing.T) {
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.ProbeHostAPI(ctx, "http://127.0.0.1:1", "probe-token"); err == nil {
		t.Fatal("probe of a closed port succeeded")
	}
}

func TestProbeHostAPIRequiresTheControlCredential(t *testing.T) {
	var seen []string
	target := newHostAPITarget(t, "probe-token", &seen)
	cs, _, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	srv := httptest.NewServer(cs.Router())
	t.Cleanup(srv.Close)
	host, port := parseHostPort(t, srv.URL)
	anonymous := agentctl.NewControlClient(host, port, logger.Default())
	if err := anonymous.ProbeHostAPI(context.Background(), target.URL, "probe-token"); err == nil {
		t.Fatal("an unauthenticated control client could drive the probe")
	}
	if len(seen) != 0 {
		t.Fatalf("an unauthenticated probe reached the target: %v", seen)
	}
}

func TestProbeHostAPIFailsClosedWithoutConfiguredCredential(t *testing.T) {
	var seen []string
	target := newHostAPITarget(t, "probe-token", &seen)
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "")

	if err := client.ProbeHostAPI(context.Background(), target.URL, "probe-token"); err == nil {
		t.Fatal("a probe without a configured control credential succeeded")
	}
	if len(seen) != 0 {
		t.Fatalf("the unauthenticated probe reached the target: %v", seen)
	}
}
