package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Sandbox) docker(ctx context.Context, args ...string) (string, int, error) {
	config := filepath.Join(s.root, "docker-client")
	if err := os.MkdirAll(config, 0700); err != nil {
		return "", -1, err
	}
	fixed := []string{"--host", "unix:///var/run/docker.sock", "--config", config}
	run := s.run
	if run == nil {
		run = runCommand
	}
	return run(ctx, "", "docker", append(fixed, args...)...)
}

func (s *Sandbox) containerCheck(ctx context.Context, dir, image string, argv []string, guard Guard) (string, int, error) {
	if len(argv) == 0 || !strings.HasPrefix(image, "sha256:") || !objectID.MatchString(strings.TrimPrefix(image, "sha256:")) || os.Geteuid() == 0 || strings.ContainsAny(dir, ",\r\n") {
		return "", -1, ErrBoundary
	}
	name := "kandev-maintenance-" + uuid.NewString()
	args := []string{"create", "--pull", "never", "--name", name, "--label", "kandev.task=orchestration-maintenance", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--memory-swap", "512m", "--cpus", "1",
		"--user", fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()), "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m", "--workdir", "/workspace",
		"--mount", "type=bind,src=" + dir + ",dst=/workspace,readonly,bind-recursive=disabled", "--env", "HOME=/tmp", "--env", "CI=true",
		"--entrypoint", argv[0], image}
	args = append(args, argv[1:]...)
	defer s.removeContainer(name)
	if err := guard(ctx); err != nil {
		return "", -1, err
	}
	if _, _, err := s.docker(ctx, args...); err != nil {
		return "", -1, fmt.Errorf("maintenance sandbox unavailable")
	}
	if err := guard(ctx); err != nil {
		return "", -1, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, _, _ := s.docker(ctx, "start", "--attach", name)
	if err := ctx.Err(); err != nil {
		return "", -1, err
	}
	state, _, err := s.docker(ctx, "inspect", "--format", "{{json .State}}", name)
	var status struct {
		Status   string
		ExitCode int
		Error    string
	}
	if err != nil || json.Unmarshal([]byte(state), &status) != nil || status.Status != "exited" || status.Error != "" {
		return "", -1, fmt.Errorf("maintenance check outcome unknown")
	}
	return out, status.ExitCode, nil
}

func (s *Sandbox) qualifySandbox(ctx context.Context, image string) error {
	dir, err := os.MkdirTemp(s.root, "probe-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err = os.WriteFile(filepath.Join(dir, "fixture"), []byte("Synthetic boundary check"), 0600); err != nil {
		return err
	}
	_, code, err := s.containerCheck(ctx, dir, image, []string{"/bin/sh", "-c", sandboxProbe}, func(context.Context) error { return nil })
	if err != nil || code != 0 {
		return fmt.Errorf("maintenance isolation could not be qualified")
	}
	return nil
}

const sandboxProbe = `set -eu
test -r /workspace/fixture
test ! -e /var/run/docker.sock
if (echo denied > /workspace/fixture) 2>/dev/null; then exit 11; fi
uid=0; caps=unknown; privileges=0
while read -r key value rest; do
  case "$key" in Uid:) uid="$value";; CapEff:) caps="$value";; NoNewPrivs:) privileges="$value";; esac
done < /proc/self/status
test "$uid" != 0
test "$caps" = 0000000000000000
test "$privileges" = 1
while IFS= read -r line; do
  case "$line" in *:*) set -- ${line%%:*}; test "$1" = lo;; esac
done < /proc/net/dev
`

func (s *Sandbox) removeContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, _ = s.docker(ctx, "rm", "--force", name)
}
