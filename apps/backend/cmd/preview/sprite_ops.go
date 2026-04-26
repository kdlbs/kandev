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
func extractBundle(ctx context.Context, sprite *sprites.Sprite) error {
	script := buildExtractScript()
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "bash", "-c", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("extract bundle: %w\n%s", err, string(out))
	}
	return nil
}

func buildExtractScript() string {
	return fmt.Sprintf(`set -e
tar -xzf /tmp/kandev-preview.tar.gz -C /
chmod +x /app/apps/backend/bin/kandev \
         /app/apps/backend/bin/agentctl \
         /app/apps/backend/bin/mock-agent
ln -sf /app/apps/backend/bin/agentctl    /usr/local/bin/agentctl
ln -sf /app/apps/backend/bin/mock-agent  /usr/local/bin/mock-agent
mkdir -p /data /var/log
cat > /app/start-kandev.sh << 'STARTSCRIPT'
#!/bin/bash
set -e
mkdir -p /data

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
exec /app/apps/backend/bin/kandev
STARTSCRIPT
chmod +x /app/start-kandev.sh`, ports.Web, ports.Backend, ports.Web)
}

// deployService registers (or re-registers) kandev as a Sprites managed service.
// Uses PUT semantics: safe to call multiple times for re-deploys.
func deployService(ctx context.Context, sprite *sprites.Sprite, port int) error {
	// Stop the service first if running; ignore errors (may not exist yet).
	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()
	if stream, err := sprite.StopService(stopCtx, "kandev"); err == nil {
		_ = drainStream(stream)
	}

	svcCtx, svcCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer svcCancel()

	stream, err := sprite.CreateService(svcCtx, "kandev", &sprites.ServiceRequest{
		Cmd:      "/app/start-kandev.sh",
		HTTPPort: &port,
	})
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = stream.Close() }()

	return waitForServiceStarted(stream)
}

func waitForServiceStarted(stream *sprites.ServiceStream) error {
	started := false
	for {
		event, err := stream.Next()
		if errors.Is(err, io.EOF) {
			if !started {
				return fmt.Errorf("service stream closed before 'started' event")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("service stream: %w", err)
		}
		switch event.Type {
		case "started":
			fmt.Fprintln(os.Stderr, "  kandev service started")
			started = true
			return nil
		case "error":
			return fmt.Errorf("service error: %s", event.Data)
		case "exit":
			code := -1
			if event.ExitCode != nil {
				code = *event.ExitCode
			}
			return fmt.Errorf("service exited (code %d) before 'started'", code)
		case "stdout", "stderr":
			if event.Data != "" {
				fmt.Fprintf(os.Stderr, "  [kandev] %s\n", strings.TrimRight(event.Data, "\n"))
			}
		}
	}
}

func drainStream(stream *sprites.ServiceStream) error {
	for {
		_, err := stream.Next()
		if errors.Is(err, io.EOF) || err != nil {
			return err
		}
	}
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
