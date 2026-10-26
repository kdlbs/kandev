package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const (
	maxRuntimeDrainDeadline = 30 * time.Second
	minRuntimeDrainDeadline = time.Millisecond
)

func (m *ControlServer) handleRuntimeSealAndDrain(c *gin.Context) {
	// Control auth historically permits unauthenticated local use when no
	// credential was configured. This operation is evidence-bearing, so it
	// fails closed until the current process credential exists.
	if m.credentials.Latest() == "" {
		c.JSON(http.StatusUnauthorized, gin.H{errKey: "runtime drain requires control credentials"})
		return
	}
	req, ok := m.decodeRuntimeDrainRequest(c)
	if !ok {
		return
	}

	deadline := time.Duration(req.DeadlineMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(c.Request.Context(), deadline)
	defer cancel()

	for {
		done, handled := m.claimRuntimeDrain(c, ctx, req)
		if handled {
			return
		}
		if done == nil {
			continue
		}
		m.runDrainAndRespond(c, ctx, req, done)
		return
	}
}

func (m *ControlServer) decodeRuntimeDrainRequest(c *gin.Context) (agentctltypes.RuntimeDrainRequest, bool) {
	var req agentctltypes.RuntimeDrainRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid runtime drain request"})
		return req, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid runtime drain request"})
		return req, false
	}
	if strings.TrimSpace(req.RequestID) != req.RequestID || req.RequestID == "" || len(req.RequestID) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "request_id must be 1-128 non-whitespace characters"})
		return req, false
	}
	if req.ExpectedServerIdentity == "" || req.ExpectedServerIdentity != m.cfg.ServerIdentity {
		c.JSON(http.StatusConflict, gin.H{errKey: "runtime identity mismatch"})
		return req, false
	}
	if req.DeadlineMS < int64(minRuntimeDrainDeadline/time.Millisecond) ||
		req.DeadlineMS > int64(maxRuntimeDrainDeadline/time.Millisecond) {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "deadline_ms must be between 1 and 30000"})
		return req, false
	}
	return req, true
}

// claimRuntimeDrain binds the request and either responds (handled=true),
// asks the caller to retry the loop (done=nil), or returns the done channel
// of a drain this caller now owns.
func (m *ControlServer) claimRuntimeDrain(c *gin.Context, ctx context.Context, req agentctltypes.RuntimeDrainRequest) (done chan struct{}, handled bool) {
	m.runtimeDrainMu.Lock()
	if m.runtimeDrainReq != nil && *m.runtimeDrainReq != req {
		m.runtimeDrainMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{errKey: "runtime already bound to another drain request"})
		return nil, true
	}
	if m.runtimeDrainReq == nil {
		copy := req
		m.runtimeDrainReq = &copy
	}
	if m.runtimeDrainAck != nil {
		receipt := *m.runtimeDrainAck
		m.runtimeDrainMu.Unlock()
		c.JSON(http.StatusOK, receipt)
		return nil, true
	}
	if m.runtimeDrainRun {
		wait := m.runtimeDrainDone
		m.runtimeDrainMu.Unlock()
		select {
		case <-wait:
			return nil, false
		case <-ctx.Done():
			c.JSON(http.StatusGatewayTimeout, gin.H{errKey: "runtime drain deadline exceeded"})
			return nil, true
		}
	}
	m.runtimeDrainRun = true
	m.runtimeDrainDone = make(chan struct{})
	done = m.runtimeDrainDone
	m.runtimeDrainMu.Unlock()
	return done, false
}

func (m *ControlServer) runDrainAndRespond(c *gin.Context, ctx context.Context, req agentctltypes.RuntimeDrainRequest, done chan struct{}) {
	m.ownership.BeginShutdown()
	err := m.ShutdownInstances(ctx)
	var receipt *agentctltypes.RuntimeDrainReceipt
	if err == nil {
		receipt = &agentctltypes.RuntimeDrainReceipt{
			RequestID:      req.RequestID,
			ServerIdentity: req.ExpectedServerIdentity,
			Sealed:         true,
			Drained:        true,
			Scope:          "managed_process_groups",
			Version:        "runtime-seal-drain.v1",
			CompletedAt:    time.Now().UTC(),
		}
	}

	m.runtimeDrainMu.Lock()
	if receipt != nil {
		m.runtimeDrainAck = receipt
	}
	m.runtimeDrainRun = false
	close(done)
	m.runtimeDrainMu.Unlock()

	if err != nil {
		if ctx.Err() != nil {
			c.JSON(http.StatusGatewayTimeout, gin.H{errKey: "runtime drain deadline exceeded"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{errKey: "managed runtime drain failed"})
		return
	}
	c.JSON(http.StatusOK, receipt)
}

// ShutdownInstances serializes runtime drain and process shutdown initiated by
// the agentctl main loop. A failed drain remains retryable; successful manager
// shutdown is memoized so a later signal cannot run teardown twice.
func (m *ControlServer) ShutdownInstances(ctx context.Context) error {
	m.ownership.BeginShutdown()
	select {
	case m.shutdownGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-m.shutdownGate }()

	m.shutdownStateMu.Lock()
	if m.shutdownComplete {
		m.shutdownStateMu.Unlock()
		return nil
	}
	m.shutdownStateMu.Unlock()

	if err := m.instMgr.Shutdown(ctx); err != nil {
		return err
	}
	m.shutdownStateMu.Lock()
	m.shutdownComplete = true
	m.shutdownStateMu.Unlock()
	return nil
}
