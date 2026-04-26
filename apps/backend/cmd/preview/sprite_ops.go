package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/ports"
	sprites "github.com/superfly/sprites-go"
)

const (
	spriteUploadTimeout = 10 * time.Minute // bundles can be large
	spriteStepTimeout   = 2 * time.Minute
	spriteUploadRetries = 3
	spriteBackoffInit   = 700 * time.Millisecond
)

func newSpriteClient(token string) *sprites.Client {
	return sprites.New(token)
}

// getOrCreateSprite returns an existing sprite or creates a new one.
// Cold/sleeping sprites wake automatically when commands are issued.
func getOrCreateSprite(ctx context.Context, client *sprites.Client, name string) (*sprites.Sprite, error) {
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	sprite, err := client.GetSprite(stepCtx, name)
	if err == nil {
		return sprite, nil
	}
	// The SDK returns a plain fmt.Errorf("sprite not found: %s") for 404s —
	// no typed error is available, so we check the message string.
	if !strings.Contains(err.Error(), "not found") {
		return nil, fmt.Errorf("get sprite: %w", err)
	}

	createCtx, createCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer createCancel()

	sprite, err = client.CreateSprite(createCtx, name, nil)
	if err != nil {
		return nil, fmt.Errorf("create sprite: %w", err)
	}
	return sprite, nil
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

# Start Next.js web server in background.
PORT=%d HOSTNAME=0.0.0.0 NODE_ENV=production \
  nohup node /app/apps/web/.next/standalone/web/server.js \
  > /var/log/kandev-web.log 2>&1 &

# Start Go backend (main process — Sprites HTTPPort proxies here).
export KANDEV_HOME_DIR=/data
export KANDEV_DOCKER_ENABLED=false
export KANDEV_LOG_LEVEL=info
export KANDEV_SERVER_PORT=%d
export KANDEV_WEB_INTERNAL_URL=http://localhost:%d
/app/apps/backend/bin/kandev > /var/log/kandev.log 2>&1
STARTSCRIPT
chmod +x /app/start-kandev.sh`, ports.Web, backendPort, ports.Web)
}

// deployService registers (or updates) the kandev service and explicitly starts
// it. The two-step approach (CreateService then StartService) ensures the
// process runs on both first deploy and re-deploy:
//   - CreateService is idempotent: creates the service config if absent, or
//     updates it if it already exists.
//   - StartService explicitly starts (or restarts) the process regardless of
//     whether it was previously running or stopped.
func deployService(ctx context.Context, sprite *sprites.Sprite, port int) error {
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

	// Explicitly start the service so it runs immediately.
	startCtx, startCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer startCancel()

	startStream, err := sprite.StartService(startCtx, "kandev")
	if err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	defer func() { _ = startStream.Close() }()

	return waitForServiceStarted(startStream)
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

// waitForKandev polls the kandev /health endpoint inside the sprite via
// CommandContext until kandev responds or the deadline is exceeded.
// Using the internal address (localhost) avoids Sprites routing state that
// may lag during a service restart triggered by enablePublicURL.
func waitForKandev(ctx context.Context, sprite *sprites.Sprite, port int) error {
	const (
		timeout   = 90 * time.Second
		retryWait = 3 * time.Second
	)
	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, err := sprite.CommandContext(checkCtx, "curl", "-sf", healthURL).Output()
		cancel()

		if err == nil && len(out) > 0 {
			fmt.Fprintf(os.Stderr, "  kandev is healthy\n")
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryWait):
		}
	}

	// Health check timed out — fetch logs to help diagnose.
	diag := fetchSpriteLogs(ctx, sprite)
	return fmt.Errorf("kandev did not become healthy within %v\n%s", timeout, diag)
}

// fetchSpriteLogs reads log files from the sprite for failure diagnostics.
func fetchSpriteLogs(ctx context.Context, sprite *sprites.Sprite) string {
	logCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	script := `echo "=== /var/log/kandev.log ==="; tail -50 /var/log/kandev.log 2>/dev/null || echo "(empty)"; echo "=== /var/log/kandev-web.log ==="; tail -20 /var/log/kandev-web.log 2>/dev/null || echo "(empty)"`
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
