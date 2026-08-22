package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

const (
	healthPollInterval = 300 * time.Millisecond
	healthProbeTimeout = 2 * time.Second
)

// healthProbeClient bounds every individual health request. http.DefaultClient
// has no timeout, so a backend that accepts the connection but never answers
// would otherwise block the launcher forever.
var healthProbeClient = &http.Client{Timeout: healthProbeTimeout}

type childState interface {
	Exited() (bool, int)
}

func healthTimeout(defaultMS int) time.Duration {
	raw := os.Getenv("KANDEV_HEALTH_TIMEOUT_MS")
	if raw == "" {
		return time.Duration(defaultMS) * time.Millisecond
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return time.Duration(defaultMS) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}

func healthTimeoutForConfig(defaultMS int, cfg *config.Config) time.Duration {
	if raw, ok := os.LookupEnv("KANDEV_HEALTH_TIMEOUT_MS"); ok && strings.TrimSpace(raw) != "" {
		return healthTimeout(defaultMS)
	}
	if configSourceIsExplicit(cfg, "launcher.healthTimeoutMs") || cfg != nil {
		if cfg != nil && cfg.Launcher.HealthTimeoutMs > 0 {
			return time.Duration(cfg.Launcher.HealthTimeoutMs) * time.Millisecond
		}
	}
	return healthTimeout(defaultMS)
}

func waitForHealth(ctx context.Context, baseURL string, proc childState, timeout time.Duration, expectedToken string, onFailure func()) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	healthURL := baseURL + "/health"
	sawForeignProcessResponse := false
	for ctx.Err() == nil {
		if exited, code := proc.Exited(); exited {
			if onFailure != nil {
				onFailure()
			}
			return fmt.Errorf("backend exited (code %d) before healthcheck passed", code)
		}
		healthy, foreign := probeHealth(ctx, healthURL, expectedToken)
		if foreign {
			sawForeignProcessResponse = true
		}
		if healthy {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(healthPollInterval):
		}
	}
	if onFailure != nil {
		onFailure()
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("backend healthcheck canceled at %s: %w", healthURL, ctx.Err())
	}
	if sawForeignProcessResponse {
		return fmt.Errorf(
			"backend port %s answered a health check from a different process "+
				"(missing/mismatched launcher token). Another Kandev instance may already own it, "+
				"or the runtime bundle predates v0.66.0",
			healthPort(baseURL),
		)
	}
	return fmt.Errorf("backend healthcheck timed out after %s at %s", timeout, healthURL)
}

// probeHealth reports whether a single health request succeeded. The body is
// drained and closed so the connection can be reused by the next poll.
func probeHealth(ctx context.Context, healthURL, expectedToken string) (healthy, foreign bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false, false
	}
	resp, err := healthProbeClient.Do(req)
	if err != nil {
		return false, false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, false
	}
	if expectedToken == "" || resp.Header.Get("X-Kandev-Desktop-Health-Token") == expectedToken {
		return true, false
	}
	return false, true
}

// waitForReady polls GET /ready until the backend reports readiness or the
// process exits. Unlike waitForHealth, it has no timeout of its own and never
// triggers a kill: /health (and its healthTimeoutReleaseMS/healthTimeoutDevMS
// budget) already made the keep-or-kill decision by the time this runs. This
// only decides when the bootstrap handler has handed off to the real router,
// so the launcher can print "backend ready"/open a browser onto real content
// instead of the bootstrap stub's 503 while startup recovery is still
// in flight. Do not fold this into waitForHealth's timeout — that would
// recreate the crash loop docs/specs/startup-listener-before-recovery/spec.md
// exists to fix.
func waitForReady(ctx context.Context, baseURL string, proc childState) error {
	readyURL := baseURL + "/ready"
	for ctx.Err() == nil {
		if exited, code := proc.Exited(); exited {
			return fmt.Errorf("backend exited (code %d) before it reported ready", code)
		}
		if probeReady(ctx, readyURL) {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(healthPollInterval):
		}
	}
	return fmt.Errorf("backend readiness wait canceled at %s: %w", readyURL, ctx.Err())
}

// probeReady reports whether a single readiness request returned 2xx. Unlike
// probeHealth, it does not check the desktop health token: readyHandler never
// sets X-Kandev-Desktop-Health-Token (that header is /health-only, see
// docs/specs/health-endpoint-version/spec.md AC-21).
func probeReady(ctx context.Context, readyURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	if err != nil {
		return false
	}
	resp, err := healthProbeClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func healthPort(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Port() != "" {
		return parsed.Port()
	}
	return baseURL
}
