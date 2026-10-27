package utility

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/sandbox"
	"net/http"
	"os"
	"os/exec"
)

func (g *utilitySandboxGate) starting() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dispatched {
		return sandbox.ErrInvalidPolicy
	}
	g.dispatched = true
	g.outcome = ""
	return nil
}

func (g *utilitySandboxGate) failedStart() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.owner == nil {
		g.outcome = sandbox.StartNotStarted
	}
}

func (g *utilitySandboxGate) receipt() *sandbox.StartReceipt {
	g.mu.Lock()
	defer g.mu.Unlock()
	receipt := &sandbox.StartReceipt{Version: 1, AttemptID: g.launch.AttemptID, Generation: g.launch.Generation, PolicyDigest: g.launch.PolicyDigest, Outcome: g.outcome}
	if g.owner != nil {
		if sandbox.RetireDescendants(context.Background(), g.launch, g.owner) != nil {
			return nil
		}
		copied := *g.owner
		receipt.Owner = &copied
		receipt.Outcome = sandbox.StartTerminated
	}
	if receipt.ValidateStopped(g.launch) != nil {
		return nil
	}
	return receipt
}

func utilitySandboxReceipt(cfg *InferenceConfigDTO) *sandbox.StartReceipt {
	if cfg == nil || cfg.SandboxReceipt == nil {
		return nil
	}
	return cfg.SandboxReceipt()
}

func startUtilitySandboxCommand(command *exec.Cmd, cfg *InferenceConfigDTO) error {
	if cfg.Sandbox == nil {
		return command.Start()
	}
	release, err := sandbox.ConfigureContainment(command, cfg.Sandbox)
	if err != nil {
		return err
	}
	defer release()
	if cfg.SandboxStarting == nil || cfg.SandboxStartFailed == nil {
		return sandbox.ErrInvalidPolicy
	}
	if err := cfg.SandboxStarting(); err != nil {
		return err
	}
	err = command.Start()
	if err != nil && command.Process == nil {
		cfg.SandboxStartFailed()
	}
	return err
}

func (h *Handler) handleSandboxReceipt(c *gin.Context) {
	if h.authenticatedControl == nil || !h.authenticatedControl(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	h.sandboxMu.Lock()
	gate := h.sandboxes[c.Param("attemptID")]
	h.sandboxMu.Unlock()
	if gate == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	gate.mu.Lock()
	completed := gate.completed
	gate.mu.Unlock()
	if !completed {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	receipt := gate.receipt()
	if receipt == nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	c.JSON(http.StatusOK, receipt)
}

// A completed attempt stays non-replayable while its immutable files exist.
func (h *Handler) pruneSandboxReceiptsLocked() {
	for key, gate := range h.sandboxes {
		gate.mu.Lock()
		completed := gate.completed
		root := gate.launch.Root
		gate.mu.Unlock()
		if !completed {
			continue
		}
		if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
			delete(h.sandboxes, key)
		}
	}
}
