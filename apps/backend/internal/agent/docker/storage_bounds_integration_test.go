package docker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestContainerTmpfsOnLocalDaemon(t *testing.T) {
	image := os.Getenv("KANDEV_DOCKER_STORAGE_CANARY_IMAGE")
	if image == "" {
		t.Skip("set KANDEV_DOCKER_STORAGE_CANARY_IMAGE to an existing local image with sh and stat")
	}
	c := newLocalStorageCanaryClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	defaultID := createStorageCanary(t, c, ctx, image, ContainerConfig{})
	defaultInfo, err := c.inspectContainer(ctx, defaultID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := defaultInfo.HostConfig.Tmpfs["/tmp"]; ok {
		t.Fatalf("default container unexpectedly has /tmp tmpfs: %#v", defaultInfo.HostConfig.Tmpfs)
	}

	optedID := createStorageCanary(t, c, ctx, image, ContainerConfig{
		TmpfsSizeBytes: 2 * 1024 * 1024 * 1024,
		Entrypoint:     []string{"/bin/sh"},
		Cmd:            []string{"-c", storageCanaryCommand},
	})
	optedInfo, err := c.inspectContainer(ctx, optedID)
	if err != nil {
		t.Fatal(err)
	}
	if optedInfo.HostConfig.Tmpfs["/tmp"] != "rw,exec,nosuid,nodev,size=2147483648,mode=1777" {
		t.Fatalf("daemon did not retain the configured /tmp tmpfs: %#v", optedInfo.HostConfig.Tmpfs)
	}
	if err := c.StartContainer(ctx, optedID); err != nil {
		t.Fatal(err)
	}
	exitCode, err := c.WaitContainer(ctx, optedID)
	if err != nil || exitCode != 0 {
		t.Fatalf("scratch probe failed: exit code %d: %v", exitCode, err)
	}
}

func TestContainerLogRotationOnCompatibleLocalDaemon(t *testing.T) {
	image := os.Getenv("KANDEV_DOCKER_STORAGE_CANARY_IMAGE")
	if image == "" {
		t.Skip("set KANDEV_DOCKER_STORAGE_CANARY_IMAGE to an existing local image")
	}
	c := newLocalStorageCanaryClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	defaultID := createStorageCanary(t, c, ctx, image, ContainerConfig{})
	defaultInfo, err := c.inspectContainer(ctx, defaultID)
	if err != nil {
		t.Fatal(err)
	}
	rotationConfig := storageCanaryConfig(image, ContainerConfig{
		LogMaxSize:  "100m",
		LogMaxFiles: "3",
	})
	rotationID, err := c.CreateContainer(ctx, rotationConfig)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unknown log opt") {
			t.Skipf("daemon logging driver %q rejects rotation options: %v", defaultInfo.HostConfig.LogConfig.Type, err)
		}
		t.Fatal(err)
	}
	cleanupStorageCanary(t, c, rotationID)
	rotationInfo, err := c.inspectContainer(ctx, rotationID)
	if err != nil {
		t.Fatal(err)
	}
	if rotationInfo.HostConfig.LogConfig.Type != defaultInfo.HostConfig.LogConfig.Type {
		t.Fatalf("rotation driver = %q, want daemon default %q", rotationInfo.HostConfig.LogConfig.Type, defaultInfo.HostConfig.LogConfig.Type)
	}
	if rotationInfo.HostConfig.LogConfig.Config["max-size"] != "100m" || rotationInfo.HostConfig.LogConfig.Config["max-file"] != "3" {
		t.Fatalf("daemon did not retain rotation options: %#v", rotationInfo.HostConfig.LogConfig)
	}
}

const storageCanaryCommand = `
set -eu
test "$(stat -f -c %T /tmp)" = tmpfs
test "$(df -k /tmp | tail -1 | awk '{print $2}')" = 2097152
printf '%s\n' '#!/bin/sh' 'echo executable-scratch-ok' > /tmp/exec-probe
chmod +x /tmp/exec-probe
test "$(/tmp/exec-probe)" = executable-scratch-ok
dd if=/dev/zero of=/tmp/probe bs=1048576 count=4 >/dev/null 2>&1
echo bounded-scratch-ok
`

func newLocalStorageCanaryClient(t *testing.T) *Client {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(config.DockerConfig{Host: "unix:///var/run/docker.sock"}, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func createStorageCanary(t *testing.T, c *Client, ctx context.Context, image string, overrides ContainerConfig) string {
	t.Helper()
	id, err := c.CreateContainer(ctx, storageCanaryConfig(image, overrides))
	if err != nil {
		t.Fatal(err)
	}
	cleanupStorageCanary(t, c, id)
	return id
}

func storageCanaryConfig(image string, overrides ContainerConfig) ContainerConfig {
	cfg := overrides
	cfg.Name = fmt.Sprintf("executor-storage-canary-%d", time.Now().UnixNano())
	cfg.Image = image
	cfg.NetworkMode = "none"
	return cfg
}

func cleanupStorageCanary(t *testing.T, c *Client, id string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := c.RemoveContainer(cleanupCtx, id, true); err != nil {
			t.Error(err)
		}
	})
}
