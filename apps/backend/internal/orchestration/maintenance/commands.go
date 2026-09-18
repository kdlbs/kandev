package maintenance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len() < 65536 {
		_, _ = b.Buffer.Write(p[:min(len(p), 65536-b.Len())])
	}
	return len(p), nil
}

func runCommand(ctx context.Context, dir, name string, args ...string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_NAME=Kandev local repair", "GIT_AUTHOR_EMAIL=repair@kandev.invalid", "GIT_COMMITTER_NAME=Kandev local repair", "GIT_COMMITTER_EMAIL=repair@kandev.invalid"}
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		code = -1
	}
	return output.String(), code, err
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	fixed := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "init.templateDir=", "-c", "commit.gpgsign=false", "-c", "protocol.allow=never", "-c", "uploadpack.packObjectsHook=", "-c", "core.autocrlf=false"}
	out, _, err := runCommand(ctx, dir, "git", append(fixed, args...)...)
	if err != nil {
		return "", errors.New("isolated git operation failed")
	}
	return strings.TrimRight(out, "\r\n"), nil
}

func stageTree(ctx context.Context, dir string, allowed []string) (string, error) {
	tracked, err := runGit(ctx, dir, "diff", "--name-only", "-z", "HEAD")
	if err != nil {
		return "", err
	}
	newFiles, err := runGit(ctx, dir, "ls-files", "--others", "-z")
	if err != nil {
		return "", err
	}
	for _, path := range strings.Split(tracked+"\x00"+newFiles, "\x00") {
		if path != "" && !slices.Contains(allowed, path) {
			return "", ErrBoundary
		}
	}
	if _, err = runGit(ctx, dir, "add", "-A", "--", "."); err != nil {
		return "", err
	}
	return runGit(ctx, dir, "write-tree")
}
