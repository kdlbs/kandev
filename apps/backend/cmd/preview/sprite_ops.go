package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"
)

const (
	spriteUploadTimeout  = 10 * time.Minute // bundles can be large
	spriteStepTimeout    = 2 * time.Minute
	spriteControlRetries = 3
	spriteUploadRetries  = 3
	spriteBackoffInit    = 700 * time.Millisecond
)

func newSpriteClient(token string) *sprites.Client {
	return sprites.New(token)
}

// getOrCreateSprite returns an existing sprite or creates a new one.
// Cold/sleeping sprites wake automatically when commands are issued.
func getOrCreateSprite(ctx context.Context, client *sprites.Client, name string) (*sprites.Sprite, error) {
	var lastErr error
	for attempt := 1; attempt <= spriteControlRetries; attempt++ {
		stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
		sprite, err := client.GetSprite(stepCtx, name)
		cancel()
		if err == nil {
			return sprite, nil
		}

		if isSpriteNotFound(err) {
			createCtx, createCancel := context.WithTimeout(ctx, spriteStepTimeout)
			sprite, err = client.CreateSprite(createCtx, name, nil)
			createCancel()
			if err == nil {
				return sprite, nil
			}
			lastErr = fmt.Errorf("create sprite: %w", err)
		} else {
			lastErr = fmt.Errorf("get sprite: %w", err)
		}

		if !isTransientSpriteError(lastErr) || attempt == spriteControlRetries {
			return nil, lastErr
		}
		if err := waitForSpriteRetry(ctx, "get/create sprite", attempt, lastErr); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func isSpriteNotFound(err error) bool {
	// The SDK returns a plain fmt.Errorf("sprite not found: %s") for 404s —
	// no typed error is available, so we check the message string.
	return strings.Contains(err.Error(), "not found")
}

func isTransientSpriteError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}
	// Temporary is deprecated on net.Error, but older transports still expose
	// it. Keep compatibility through a local interface without calling the
	// deprecated method on net.Error directly.
	var temporaryErr interface{ Temporary() bool }
	if errors.As(err, &temporaryErr) && temporaryErr.Temporary() {
		return true
	}

	var apiErr *sprites.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return false
}

func waitForSpriteRetry(ctx context.Context, operation string, attempt int, err error) error {
	delay := spriteBackoffInit * time.Duration(1<<(attempt-1))
	var apiErr *sprites.APIError
	if errors.As(err, &apiErr) && apiErr.GetRetryAfterSeconds() > 0 {
		delay = time.Duration(apiErr.GetRetryAfterSeconds()) * time.Second
	}

	fmt.Fprintf(os.Stderr, "  %s attempt %d failed (%v), retrying in %v...\n", operation, attempt, err, delay)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// uploadBundle uploads the bundle tarball to the sprite via the Filesystem API.
// Retries up to spriteUploadRetries times on transient errors with context-aware backoff.
func uploadBundle(ctx context.Context, sprite *sprites.Sprite, tarPath string) error {
	data, err := os.ReadFile(tarPath)
	if err != nil {
		return fmt.Errorf("read bundle: %w", err)
	}

	uploadCtx, cancel := context.WithTimeout(ctx, spriteUploadTimeout)
	defer cancel()

	backoff := spriteBackoffInit
	var lastErr error
	for attempt := 1; attempt <= spriteUploadRetries; attempt++ {
		err := sprite.Filesystem().WriteFileContext(uploadCtx, "/tmp/kandev-preview.tar.gz", data, 0o644)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == spriteUploadRetries || uploadCtx.Err() != nil {
			break
		}
		fmt.Fprintf(os.Stderr, "  upload attempt %d failed (%v), retrying in %v...\n", attempt, err, backoff)
		select {
		case <-uploadCtx.Done():
			return uploadCtx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return fmt.Errorf("upload bundle after %d attempts: %w", spriteUploadRetries, lastErr)
}

// extractBundle extracts the bundle tarball and writes the startup script.
func extractBundle(ctx context.Context, sprite *sprites.Sprite, port int) error {
	script := buildExtractScript(port)
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "bash", "-c", script).CombinedOutput()
	if len(out) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", string(out))
	}
	if err != nil {
		return fmt.Errorf("extract bundle: %w", err)
	}
	return nil
}

func buildExtractScript(backendPort int) string {
	return fmt.Sprintf(`set -e
tar -xzf /tmp/kandev-preview.tar.gz -C /
chmod +x /app/apps/backend/bin/kandev \
         /app/apps/backend/bin/agentctl \
         /app/apps/backend/bin/mock-agent
ln -sf /app/apps/backend/bin/agentctl    /usr/local/bin/agentctl
ln -sf /app/apps/backend/bin/mock-agent  /usr/local/bin/mock-agent
ln -sf /app/apps/backend/bin/kandev      /usr/local/bin/kandev
# Reset data directory on each deploy so the DB starts fresh (preview env only).
rm -rf /data
mkdir -p /data /var/log
echo "=== kandev binary info ==="
ldd /app/apps/backend/bin/kandev 2>&1 || true
echo "==========================="
cat > /app/start-kandev.sh << 'STARTSCRIPT'
#!/bin/bash
set -e
mkdir -p /data

# Kill any agentctl orphans from previous runs.
pkill -f agentctl || true
sleep 1
cd /app

export KANDEV_HOME_DIR=/data
export KANDEV_DOCKER_ENABLED=false
export KANDEV_LOG_LEVEL=info
export KANDEV_WEB_TITLE_PREFIX=Preview
export KANDEV_WEB_DIST_DIR=/app/apps/web/dist
# Preview mode: keep the mock agent while loading the built-in agent catalogue
# so Settings can show agents that are available to install.
export KANDEV_MOCK_AGENT=true
# The preview service runs from /app, so the backend's relative dist probes do
# not reach the packaged Vite build under /app/apps/web/dist.
export KANDEV_WEB_DIST_DIR=/app/apps/web/dist

# Launch through the CLI so the backend runs under the restart supervisor.
exec kandev start \
  --backend-port %d \
  --verbose \
  --headless \
  > /var/log/kandev.log 2>&1
STARTSCRIPT
chmod +x /app/start-kandev.sh`, backendPort)
}

// deployService stops any running kandev service, registers (or updates) its
// config, then explicitly starts it. Three-step stop → create → start ensures
// the process is restarted on re-deploy and started on first deploy:
//   - StopService terminates any running instance (no-op if none exists).
//   - CreateService is idempotent: creates or updates the service config.
//   - StartService explicitly starts the process regardless of prior state.
func deployService(ctx context.Context, sprite *sprites.Sprite, port int) error {
	// Stop any running instance so StartService always spawns a fresh process.
	// Errors here are best-effort — the service may not exist yet on first deploy.
	stopCtx, stopCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer stopCancel()
	if stream, err := sprite.StopService(stopCtx, "kandev"); err == nil {
		drainStream(stream)
	}

	createCtx, createCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer createCancel()

	createStream, err := sprite.CreateService(createCtx, "kandev", &sprites.ServiceRequest{
		Cmd:      "/app/start-kandev.sh",
		HTTPPort: &port,
	})
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	// Drain create stream (informational only).
	for {
		ev, err := createStream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = createStream.Close()
			return fmt.Errorf("create service stream: %w", err)
		}
		if ev.Type == "error" {
			_ = createStream.Close()
			return fmt.Errorf("create service error: %s", ev.Data)
		}
	}
	_ = createStream.Close()

	// Explicitly start the fresh service.
	startCtx, startCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer startCancel()

	startStream, err := sprite.StartService(startCtx, "kandev")
	if err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	defer func() { _ = startStream.Close() }()

	return waitForServiceStarted(startStream)
}

// drainStream reads all events from a ServiceStream and closes it.
// Used for informational streams where we don't need to act on the content.
func drainStream(stream *sprites.ServiceStream) {
	defer func() { _ = stream.Close() }()
	for {
		_, err := stream.Next()
		if err != nil {
			return
		}
	}
}

func waitForServiceStarted(stream *sprites.ServiceStream) error {
	// CreateService returns HTTP 200 when the service is registered; the stream
	// carries optional progress events. EOF means the server finished streaming —
	// treat it as success unless we saw an explicit failure event first.
	// On existing sprites the service can start fast enough that no events arrive.
	for {
		event, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("service stream: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  [service] type=%s data=%q\n", event.Type, event.Data)
		switch event.Type {
		case "started", "complete":
			return nil
		case "error":
			return fmt.Errorf("service error: %s", event.Data)
		case "exit":
			code := -1
			if event.ExitCode != nil {
				code = *event.ExitCode
			}
			return fmt.Errorf("service exited (code %d) before 'started'", code)
		}
	}
}

// kandevReadyURL returns the readiness endpoint waitForKandev polls, broken
// out so tests can pin the endpoint without going through the sprite exec/
// websocket machinery. /health only proves the bootstrap listener has bound
// the socket (it returns 200 immediately, before the real router is wired
// in); /ready is what flips once the real router is swapped in and requests
// stop getting the bootstrap handler's 503 "starting" response.
func kandevReadyURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/ready", port)
}

// waitForKandev polls the kandev /ready endpoint inside the sprite via
// CommandContext until kandev is ready to serve real traffic or the deadline
// is exceeded. Using the internal address (localhost) avoids Sprites routing
// state that may lag during a service restart triggered by enablePublicURL.
func waitForKandev(ctx context.Context, sprite *sprites.Sprite, port int) error {
	const (
		timeout   = 90 * time.Second
		retryWait = 3 * time.Second
	)
	readyURL := kandevReadyURL(port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, err := sprite.CommandContext(checkCtx, "curl", "-sf", readyURL).Output()
		cancel()

		if err == nil && len(out) > 0 {
			fmt.Fprintf(os.Stderr, "  kandev is ready\n")
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryWait):
		}
	}

	// Readiness check timed out — fetch logs to help diagnose.
	diag := fetchSpriteLogs(ctx, sprite)
	return fmt.Errorf("kandev did not become ready within %v\n%s", timeout, diag)
}

// fetchSpriteLogs reads log files from the sprite for failure diagnostics.
func fetchSpriteLogs(ctx context.Context, sprite *sprites.Sprite) string {
	logCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	script := `echo "=== /var/log/kandev.log ==="; tail -50 /var/log/kandev.log 2>/dev/null || echo "(empty)"`
	out, err := sprite.CommandContext(logCtx, "bash", "-c", script).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("[log fetch error: %v]\n%s", err, string(out))
	}
	return string(out)
}

// destroySprite destroys the named sprite and returns its creation time for
// runtime calculation. Returns zero time if the sprite was not found.
func destroySprite(ctx context.Context, client *sprites.Client, name string) (time.Time, error) {
	getCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	sprite, err := client.GetSprite(getCtx, name)
	if err != nil {
		// The SDK returns a plain fmt.Errorf("sprite not found: %s") for 404s.
		if strings.Contains(err.Error(), "not found") {
			fmt.Fprintf(os.Stderr, "sprite %s not found, skipping destroy\n", name)
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("get sprite: %w", err)
	}
	createdAt := sprite.CreatedAt

	destroyCtx, destroyCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer destroyCancel()

	if err := sprite.Delete(destroyCtx); err != nil {
		return createdAt, fmt.Errorf("delete sprite: %w", err)
	}
	fmt.Fprintf(os.Stderr, "sprite %s destroyed\n", name)
	return createdAt, nil
}
