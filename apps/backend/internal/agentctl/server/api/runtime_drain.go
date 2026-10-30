package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const (
	maxRuntimeDrainDeadline = 60 * time.Second
	minRuntimeDrainDeadline = time.Millisecond
	maxRuntimeDrainBodySize = 4096
	maxRuntimeDrainRequest  = 128
)

// runtimeDrainState serializes seal-and-drain. One request owns a running
// drain; the same request retried waits for it, a different request during it
// is refused, and once the runtime is sealed any request id is answered with
// its own receipt without stopping anything again.
type runtimeDrainState struct {
	mu      sync.Mutex
	running string
	done    chan struct{}
	sealed  bool
	acks    map[string]agentctltypes.RuntimeDrainReceipt
}

func (m *ControlServer) handleRuntimeSealAndDrain(c *gin.Context) {
	// Control auth permits unauthenticated local use when no credential was
	// configured. A receipt is evidence, so it requires a credential.
	if m.credentials.Latest() == "" {
		c.JSON(http.StatusUnauthorized, gin.H{errKey: "runtime drain requires control credentials"})
		return
	}
	req, ok := m.decodeRuntimeDrainRequest(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(req.DeadlineMS)*time.Millisecond)
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
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxRuntimeDrainBodySize))
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
	if strings.TrimSpace(req.RequestID) != req.RequestID || req.RequestID == "" || len(req.RequestID) > maxRuntimeDrainRequest {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "request_id must be 1-128 non-whitespace characters"})
		return req, false
	}
	if req.ExpectedServerIdentity == "" || req.ExpectedServerIdentity != m.cfg.ServerIdentity {
		c.JSON(http.StatusPreconditionFailed, gin.H{errKey: "runtime identity mismatch"})
		return req, false
	}
	if req.DeadlineMS < int64(minRuntimeDrainDeadline/time.Millisecond) ||
		req.DeadlineMS > int64(maxRuntimeDrainDeadline/time.Millisecond) {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "deadline_ms must be between 1 and 60000"})
		return req, false
	}
	return req, true
}

func runtimeDrainReceipt(req agentctltypes.RuntimeDrainRequest) agentctltypes.RuntimeDrainReceipt {
	return agentctltypes.RuntimeDrainReceipt{
		RequestID:      req.RequestID,
		ServerIdentity: req.ExpectedServerIdentity,
		Sealed:         true,
		Drained:        true,
		Scope:          agentctltypes.RuntimeDrainScope,
		Version:        agentctltypes.RuntimeDrainVersion,
		CompletedAt:    time.Now().UTC(),
	}
}

// claimRuntimeDrain either responds (handled), asks the caller to loop again
// after a drain it waited on ended (done == nil), or hands back the done
// channel of a drain this caller now owns.
func (m *ControlServer) claimRuntimeDrain(c *gin.Context, ctx context.Context, req agentctltypes.RuntimeDrainRequest) (done chan struct{}, handled bool) {
	st := &m.runtimeDrain
	st.mu.Lock()
	if receipt, ok := st.acks[req.RequestID]; ok {
		st.mu.Unlock()
		c.JSON(http.StatusOK, receipt)
		return nil, true
	}
	if st.sealed {
		receipt := runtimeDrainReceipt(req)
		st.acks[req.RequestID] = receipt
		st.mu.Unlock()
		c.JSON(http.StatusOK, receipt)
		return nil, true
	}
	if st.running != "" {
		if st.running != req.RequestID {
			st.mu.Unlock()
			c.JSON(http.StatusConflict, gin.H{errKey: "runtime drain already running for another request"})
			return nil, true
		}
		wait := st.done
		st.mu.Unlock()
		select {
		case <-wait:
			return nil, false
		case <-ctx.Done():
			c.JSON(http.StatusGatewayTimeout, gin.H{errKey: "runtime drain deadline exceeded"})
			return nil, true
		}
	}
	st.running = req.RequestID
	st.done = make(chan struct{})
	done = st.done
	st.mu.Unlock()
	return done, false
}

func (m *ControlServer) runDrainAndRespond(c *gin.Context, ctx context.Context, req agentctltypes.RuntimeDrainRequest, done chan struct{}) {
	err := m.ShutdownInstances(ctx)

	st := &m.runtimeDrain
	st.mu.Lock()
	var receipt agentctltypes.RuntimeDrainReceipt
	if err == nil {
		receipt = runtimeDrainReceipt(req)
		st.sealed = true
		st.acks[req.RequestID] = receipt
	}
	st.running = ""
	close(done)
	st.mu.Unlock()

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

// ShutdownInstances serializes runtime drain and the process shutdown started
// by the agentctl main loop. A failed or timed-out drain stays retryable; a
// successful one is memoized so a later signal cannot tear down twice.
func (m *ControlServer) ShutdownInstances(ctx context.Context) error {
	m.ownership.BeginShutdown()
	select {
	case m.shutdownGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-m.shutdownGate }()

	m.shutdownStateMu.Lock()
	complete := m.shutdownComplete
	m.shutdownStateMu.Unlock()
	if complete {
		return nil
	}
	if err := m.instMgr.Shutdown(ctx); err != nil {
		return err
	}
	m.shutdownStateMu.Lock()
	m.shutdownComplete = true
	m.shutdownStateMu.Unlock()
	return nil
}
