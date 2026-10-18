package lsp

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/lsp/installer"
)

// installCoordinator serializes cache mutations that share an installer
// prefix or binary target. Waiting task/language starts remain cancellable.
type installCoordinator struct {
	mu     sync.Mutex
	active map[string]chan struct{}
}

func newInstallCoordinator() *installCoordinator {
	return &installCoordinator{active: make(map[string]chan struct{})}
}

func (c *installCoordinator) run(ctx context.Context, key string, install func() (string, error)) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		c.mu.Lock()
		wait, busy := c.active[key]
		if !busy {
			wait = make(chan struct{})
			c.active[key] = wait
			c.mu.Unlock()
			defer c.release(key, wait)
			return install()
		}
		c.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return "", context.Cause(ctx)
		}
	}
}

func (c *installCoordinator) release(key string, completed chan struct{}) {
	c.mu.Lock()
	if c.active[key] == completed {
		delete(c.active, key)
		close(completed)
	}
	c.mu.Unlock()
}

func installMutationKey(language string) string {
	binary, _ := installer.LspCommand(language)
	switch language {
	case languageTypeScript, languagePython:
		return "npm-prefix"
	case "go":
		return "go:" + binary
	case languageRust:
		return "release:" + binary
	default:
		return "language:" + language
	}
}

var sharedInstallCoordinator = newInstallCoordinator()
