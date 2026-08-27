package launcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

// goosWindows is runtime.GOOS's value for Windows, hoisted so tests and the
// Windows shim branch share one spelling. cmdExe is the cmd.exe launcher used
// to run .cmd shims and to open URLs on Windows.
const (
	goosWindows = "windows"
	cmdExe      = "cmd.exe"
)

// webCommand returns the command and args that start the Vite dev server.
// On Windows, pnpm installs as a .cmd shim that cannot be spawned directly
// (Node's spawn ignores PATHEXT, and direct .cmd spawning returns EINVAL on
// modern Node), so it goes through cmd.exe /c like the TypeScript launcher's
// resolveWindowsShim. The webCommandFor indirection lets tests exercise the
// Windows branch on any platform.
func webCommand() (string, []string) {
	return webCommandFor(runtime.GOOS)
}

func webCommandFor(goos string) (string, []string) {
	args := []string{"-C", "apps", "--filter", "@kandev/web", "dev"}
	if goos == goosWindows {
		return cmdExe, append([]string{"/c", "pnpm"}, args...)
	}
	return "pnpm", args
}

// buildWebEnv assembles the Vite dev-server environment. Browser traffic
// enters through the Go backend in every mode, so frontend code should use
// same-origin API/WS URLs: KANDEV_API_BASE_URL points the web app at the
// backend port.
//
// Any inherited VITE_KANDEV_API_PORT is explicitly removed so a host-level
// variable (from a .env file, Docker env, or CI) cannot leak through and
// reintroduce cross-origin browser API calls.
func buildWebEnv(ports portConfig, debug bool) ([]string, error) {
	if ports.WebPort == 0 {
		return nil, fmt.Errorf("web env requires a web port")
	}
	env := os.Environ()
	env = upsertEnv(env, "KANDEV_API_BASE_URL", ports.BackendURL)
	env = upsertEnv(env, "PORT", fmt.Sprint(ports.WebPort))
	env = upsertEnv(env, "HOSTNAME", "127.0.0.1")
	env = removeEnv(env, "VITE_KANDEV_API_PORT")
	if debug {
		env = upsertEnv(env, "KANDEV_DEBUG", "true")
		env = upsertEnv(env, "VITE_KANDEV_DEBUG", "true")
	}
	return env, nil
}

// removeEnv drops every item with the given key from the environment slice.
func removeEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0]
	for _, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			continue
		}
		out = append(out, item)
	}
	return out
}

// waitForURLReady polls url until any HTTP response arrives. Unlike
// waitForHealth it requires no launcher health token: the Vite dev server is
// not a Kandev process and does not emit one. Any HTTP status counts as
// ready; a connection error just keeps polling. If the child exits first the
// wait fails fast with the child's exit code.
func waitForURLReady(ctx context.Context, url string, proc childState, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for ctx.Err() == nil {
		if exited, code := proc.Exited(); exited {
			return fmt.Errorf("web process exited (code %d) before URL became reachable", code)
		}
		if urlResponds(ctx, url) {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(healthPollInterval):
		}
	}
	return fmt.Errorf("web URL readiness timed out after %s at %s", timeout, url)
}

// urlResponds reports whether a single GET to url produces any HTTP response.
// The body is drained and closed so the connection can be reused by the next
// poll.
func urlResponds(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := healthProbeClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return true
}
