package mcpconfig

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

const cursorAccessTokenMaxBytes = 8192

func readNativeCursorAccessToken(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "cursor-access-token", "-a", "cursor-user", "-w")
	output := &boundedCursorTokenOutput{}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil || output.exceeded {
		return "", errors.New("cursor account credential unavailable")
	}
	token := strings.TrimSpace(output.buffer.String())
	if token == "" {
		return "", errors.New("cursor account credential unavailable")
	}
	return token, nil
}

type boundedCursorTokenOutput struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (o *boundedCursorTokenOutput) Write(data []byte) (int, error) {
	remaining := cursorAccessTokenMaxBytes - o.buffer.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = o.buffer.Write(data[:remaining])
		}
		o.exceeded = true
		return len(data), nil
	}
	return o.buffer.Write(data)
}
