package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
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

func waitForHealth(ctx context.Context, baseURL string, proc childState, timeout time.Duration, onFailure func()) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	healthURL := baseURL + "/health"
	for ctx.Err() == nil {
		if exited, code := proc.Exited(); exited {
			if onFailure != nil {
				onFailure()
			}
			return fmt.Errorf("backend exited (code %d) before healthcheck passed", code)
		}
		if probeHealth(ctx, healthURL) {
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
	return fmt.Errorf("backend healthcheck timed out after %s at %s", timeout, healthURL)
}

// probeHealth reports whether a single health request succeeded. The body is
// drained and closed so the connection can be reused by the next poll.
func probeHealth(ctx context.Context, healthURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
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
