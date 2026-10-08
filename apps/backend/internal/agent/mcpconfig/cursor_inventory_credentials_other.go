//go:build !darwin

package mcpconfig

import (
	"context"
	"errors"
)

func readNativeCursorAccessToken(context.Context) (string, error) {
	return "", errors.New("cursor account credential store unsupported")
}
