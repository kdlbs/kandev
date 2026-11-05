package utility

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/sandbox"
)

type utilitySandboxGate struct {
	launch       *sandbox.Launch
	dispatched   bool
	completed    bool
	outcome      string
	mu           sync.Mutex
	owner        *sandbox.ProcessOwner
	accepted     chan struct{}
	acknowledged bool
	ctx          context.Context
}

func (h *Handler) reserveSandbox(c *gin.Context, cfg *InferenceConfigDTO) (func(), bool) {
	if cfg == nil || cfg.Sandbox == nil {
		return func() {}, true
	}
	key := cfg.Sandbox.AttemptID
	gate := &utilitySandboxGate{launch: cfg.Sandbox.Clone(), outcome: sandbox.StartNotStarted, accepted: make(chan struct{}), ctx: c.Request.Context()}
	h.sandboxMu.Lock()
	if h.sandboxes == nil {
		h.sandboxes = map[string]*utilitySandboxGate{}
	}
	if h.sandboxes[key] != nil {
		h.sandboxMu.Unlock()
		c.AbortWithStatus(http.StatusConflict)
		return nil, false
	}
	h.pruneSandboxReceiptsLocked()
	if len(h.sandboxes) >= 256 {
		h.sandboxMu.Unlock()
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return nil, false
	}
	h.sandboxes[key] = gate
	h.sandboxMu.Unlock()
	cfg.SandboxStarted = gate.started
	cfg.SandboxStarting = gate.starting
	cfg.SandboxStartFailed = gate.failedStart
	cfg.SandboxReceipt = gate.receipt
	return func() { gate.mu.Lock(); gate.completed = true; gate.mu.Unlock() }, true
}

func (g *utilitySandboxGate) started(parent context.Context, owner *sandbox.ProcessOwner) error {
	g.mu.Lock()
	if !g.dispatched || g.owner != nil {
		g.mu.Unlock()
		return sandbox.ErrInvalidPolicy
	}
	copied := *owner
	g.owner = &copied
	g.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		return sandbox.ErrInvalidPolicy
	case <-g.accepted:
		return ctx.Err()
	}
}

func (h *Handler) sandboxGate(c *gin.Context) *utilitySandboxGate {
	if h.authenticatedControl == nil || !h.authenticatedControl(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return nil
	}
	h.sandboxMu.Lock()
	gate := h.sandboxes[c.Param("attemptID")]
	h.sandboxMu.Unlock()
	if gate == nil || gate.ctx.Err() != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return nil
	}
	return gate
}

func (h *Handler) handleSandboxOwner(c *gin.Context) {
	gate := h.sandboxGate(c)
	if gate == nil {
		return
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.owner == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	owner, err := gate.owner.LiveSnapshot()
	if err != nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	c.JSON(http.StatusOK, owner)
}

func (h *Handler) handleSandboxOwnerAck(c *gin.Context) {
	gate := h.sandboxGate(c)
	if gate == nil {
		return
	}
	var owner sandbox.ProcessOwner
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&owner) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.owner == nil || owner != *gate.owner {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	if _, err := owner.LiveSnapshot(); err != nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	if !gate.acknowledged {
		gate.acknowledged = true
		close(gate.accepted)
	}
	c.Status(http.StatusNoContent)
}

func bindUtilitySandbox(ctx context.Context, cmd *exec.Cmd, cfg *InferenceConfigDTO) error {
	if cfg.Sandbox == nil {
		return nil
	}
	if cfg.SandboxStarted == nil {
		return sandbox.ErrInvalidPolicy
	}
	owner, err := sandbox.CaptureProcessOwner(cfg.Sandbox, cmd.Process.Pid)
	if err != nil {
		return err
	}
	return cfg.SandboxStarted(ctx, owner)
}
