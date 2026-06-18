package launcher

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

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

func waitForHealth(baseURL string, proc childState, timeout time.Duration, onFailure func()) error {
	deadline := time.Now().Add(timeout)
	healthURL := baseURL + "/health"
	for time.Now().Before(deadline) {
		if exited, code := proc.Exited(); exited {
			if onFailure != nil {
				onFailure()
			}
			return fmt.Errorf("backend exited (code %d) before healthcheck passed", code)
		}
		resp, err := http.Get(healthURL) //nolint:gosec,noctx
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if onFailure != nil {
		onFailure()
	}
	return fmt.Errorf("backend healthcheck timed out after %s at %s", timeout, healthURL)
}
