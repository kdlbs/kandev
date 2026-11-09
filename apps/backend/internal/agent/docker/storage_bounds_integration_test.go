package docker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestContainerStorageBoundsOnLocalDaemon(t *testing.T) {
	image := os.Getenv("KANDEV_DOCKER_STORAGE_CANARY_IMAGE")
	if image == "" {
		t.Skip("set KANDEV_DOCKER_STORAGE_CANARY_IMAGE to an existing local image with sh and stat")
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(config.DockerConfig{Host: "unix:///var/run/docker.sock"}, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := c.CreateContainer(ctx, ContainerConfig{
		Name:       fmt.Sprintf("executor-storage-canary-%d", time.Now().UnixNano()),
		Image:      image,
		Entrypoint: []string{"/bin/sh"},
		Cmd: []string{"-c", `
set -eu
test "$(stat -f -c %T /tmp)" = tmpfs
test "$(df -k /tmp | tail -1 | awk '{print $2}')" = 2097152
printf '%s\n' '#!/bin/sh' 'echo executable-scratch-ok' > /tmp/exec-probe
chmod +x /tmp/exec-probe
test "$(/tmp/exec-probe)" = executable-scratch-ok
dd if=/dev/zero of=/tmp/probe bs=1048576 count=4 >/dev/null 2>&1
echo bounded-scratch-ok
`},
		NetworkMode: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := c.RemoveContainer(cleanupCtx, id, true); err != nil {
			t.Error(err)
		}
	})
	inspected, err := c.inspectContainer(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.HostConfig.LogConfig.Config["max-size"] != "100m" || inspected.HostConfig.LogConfig.Config["max-file"] != "3" {
		t.Fatalf("daemon did not retain log budget: %#v", inspected.HostConfig.LogConfig)
	}
	if err := c.StartContainer(ctx, id); err != nil {
		t.Fatal(err)
	}
	exitCode, err := c.WaitContainer(ctx, id)
	if err != nil || exitCode != 0 {
		t.Fatalf("scratch probe failed: exit code %d: %v", exitCode, err)
	}
}
