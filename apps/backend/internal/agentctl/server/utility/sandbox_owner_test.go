//go:build linux

package utility

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSandboxUtilityWaitsForOwnershipBeforeACP(t *testing.T) {
	for _, cancelBeforeAck := range []bool{false, true} {
		t.Run(map[bool]string{false: "acknowledge", true: "cancel"}[cancelBeforeAck], func(t *testing.T) {
			cfg, _ := genericWrapperFixture(t)
			utilitySandboxFixture(t, cfg)
			handler := NewHandler("", zap.NewNop())
			handler.SetControlAuthorization(func(c *gin.Context) bool { return c.GetHeader("X-Test-Control") == "fixture" })
			router := gin.New()
			handler.RegisterRoutes(router.Group("/api/v1"))
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			body, err := json.Marshal(ProbeRequest{AgentID: "mock-agent", InferenceConfig: cfg})
			require.NoError(t, err)
			request, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/v1/inference/probe", bytes.NewReader(body))
			require.NoError(t, err)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Control", "fixture")
			finished := make(chan error, 1)
			go func() {
				response, e := server.Client().Do(request)
				if e == nil {
					var result ProbeResponse
					e = json.NewDecoder(response.Body).Decode(&result)
					_ = response.Body.Close()
					if e == nil && result.Success {
						e = result.SandboxReceipt.ValidateStopped(cfg.Sandbox)
					}
					if e == nil && !result.Success {
						e = io.ErrUnexpectedEOF
					}
				}
				finished <- e
			}()
			route := server.URL + "/api/v1/inference/sandbox/" + cfg.Sandbox.AttemptID + "/owner"
			var owner sandbox.ProcessOwner
			require.Eventually(t, func() bool {
				req, e := http.NewRequestWithContext(ctx, "GET", route, nil)
				if e != nil {
					return false
				}
				req.Header.Set("X-Test-Control", "fixture")
				response, e := server.Client().Do(req)
				if e != nil {
					return false
				}
				defer func() { _ = response.Body.Close() }()
				return response.StatusCode == 200 && json.NewDecoder(response.Body).Decode(&owner) == nil
			}, 3*time.Second, 10*time.Millisecond)
			require.Equal(t, cfg.Sandbox.AttemptID, owner.AttemptID)
			require.Equal(t, owner.Supervisor.PID, owner.Supervisor.SessionID)
			require.NotEmpty(t, owner.Supervisor.BirthToken)
			select {
			case <-finished:
				t.Fatal("ACP completed before ownership acknowledgement")
			default:
			}
			response, err := server.Client().Get(route)
			require.NoError(t, err)
			require.Equal(t, http.StatusForbidden, response.StatusCode)
			require.NoError(t, response.Body.Close())
			if cancelBeforeAck {
				cancel()
				require.Error(t, <-finished)
				require.Eventually(t, func() bool { _, e := owner.LiveSnapshot(); return e != nil }, time.Second, 10*time.Millisecond)
				return
			}
			stale := owner
			stale.Generation++
			require.Equal(t, http.StatusConflict, ackUtilityOwner(t, server, route, stale))
			require.Equal(t, http.StatusNoContent, ackUtilityOwner(t, server, route, owner))
			require.NoError(t, <-finished)
			require.Eventually(t, func() bool { _, e := owner.LiveSnapshot(); return e != nil }, time.Second, 10*time.Millisecond)
		})
	}
}

func ackUtilityOwner(t *testing.T, server *httptest.Server, route string, owner sandbox.ProcessOwner) int {
	t.Helper()
	raw, err := json.Marshal(owner)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), "PUT", route, bytes.NewReader(raw))
	require.NoError(t, err)
	request.Header.Set("X-Test-Control", "fixture")
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode
}

func TestSandboxUtilityFailedSpawnReturnsExactNoProcessReceipt(t *testing.T) {
	cfg, _ := genericWrapperFixture(t)
	utilitySandboxFixture(t, cfg)
	cfg.WorkDir = filepath.Join(t.TempDir(), "missing-workdir")
	handler := NewHandler("", zap.NewNop())
	handler.SetControlAuthorization(func(c *gin.Context) bool { return c.GetHeader("X-Test-Control") == "fixture" })
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1"))
	body, err := json.Marshal(ProbeRequest{AgentID: "mock-agent", InferenceConfig: cfg})
	require.NoError(t, err)
	request := httptest.NewRequest("POST", "/api/v1/inference/probe", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Control", "fixture")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response ProbeResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.NotNil(t, response.SandboxReceipt)
	require.Equal(t, sandbox.StartNotStarted, response.SandboxReceipt.Outcome)
	require.NoError(t, response.SandboxReceipt.ValidateStopped(cfg.Sandbox))
	replayed := httptest.NewRequest("POST", "/api/v1/inference/probe", bytes.NewReader(body))
	replayed.Header.Set("Content-Type", "application/json")
	replayed.Header.Set("X-Test-Control", "fixture")
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replayed)
	require.Equal(t, http.StatusConflict, replayResponse.Code, "an already completed attempt cannot spawn again")
	receiptRequest := httptest.NewRequest("GET", "/api/v1/inference/sandbox/"+cfg.Sandbox.AttemptID+"/receipt", nil)
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, receiptRequest)
	require.Equal(t, http.StatusForbidden, unauthorized.Code)
	receiptRequest.Header.Set("X-Test-Control", "fixture")
	receiptResponse := httptest.NewRecorder()
	router.ServeHTTP(receiptResponse, receiptRequest)
	require.Equal(t, http.StatusOK, receiptResponse.Code)
	var retained sandbox.StartReceipt
	require.NoError(t, json.Unmarshal(receiptResponse.Body.Bytes(), &retained))
	require.Equal(t, *response.SandboxReceipt, retained)
}

func TestSandboxUtilityUnknownSpawnCannotIssueCleanupReceipt(t *testing.T) {
	gate := &utilitySandboxGate{launch: &sandbox.Launch{AttemptID: "attempt", Generation: 1, PolicyDigest: "digest"}, outcome: sandbox.StartNotStarted}
	require.NoError(t, gate.starting())
	require.Nil(t, gate.receipt(), "an attempted spawn without ownership evidence remains uncertain")
	require.Error(t, gate.starting(), "one attempt cannot start two processes")
	gate.failedStart()
	require.Equal(t, sandbox.StartNotStarted, gate.receipt().Outcome)
	require.Error(t, gate.starting(), "failed spawn does not permit replay")
}

func TestSandboxUtilityReceiptRetentionIsBoundedUntilArtifactsAreRemoved(t *testing.T) {
	cfg, _ := genericWrapperFixture(t)
	utilitySandboxFixture(t, cfg)
	cfg.WorkDir = filepath.Join(t.TempDir(), "missing-workdir")
	handler := NewHandler("", zap.NewNop())
	handler.SetControlAuthorization(func(*gin.Context) bool { return true })
	historyRoot := t.TempDir()
	handler.sandboxes = map[string]*utilitySandboxGate{}
	for index := range 256 {
		handler.sandboxes[fmt.Sprint(index)] = &utilitySandboxGate{launch: &sandbox.Launch{Root: historyRoot}, completed: index != 0}
	}
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1"))
	body, err := json.Marshal(ProbeRequest{AgentID: "mock-agent", InferenceConfig: cfg})
	require.NoError(t, err)
	send := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "/api/v1/inference/probe", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	require.Equal(t, http.StatusServiceUnavailable, send().Code)
	require.NoError(t, os.RemoveAll(historyRoot))
	require.Equal(t, http.StatusOK, send().Code, "completed records can be released after core removes their artifacts")
	require.Len(t, handler.sandboxes, 2, "the unfinished record must remain even when its files disappear")
}

func TestSandboxUtilityReceiptContainsDetachedChildren(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("requires setsid fixture")
	}
	cfg, _ := genericWrapperFixture(t)
	utilitySandboxFixture(t, cfg)
	command := exec.Command("sh", "-c", "setsid sleep 30 & printf ready; read line")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := command.StdinPipe()
	require.NoError(t, err)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	release, err := sandbox.ConfigureContainment(command, cfg.Sandbox)
	require.NoError(t, err)
	require.NoError(t, command.Start())
	release()
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	owner, err := sandbox.CaptureProcessOwner(cfg.Sandbox, command.Process.Pid)
	require.NoError(t, err)
	ready := make([]byte, 5)
	_, err = io.ReadFull(output, ready)
	require.NoError(t, err)
	require.Equal(t, "ready", string(ready))
	_, err = io.WriteString(input, "finish\n")
	require.NoError(t, err)
	require.NoError(t, input.Close())
	require.NoError(t, command.Wait())
	empty, err := cfg.Sandbox.Containment.Empty()
	require.NoError(t, err)
	require.False(t, empty)
	gate := &utilitySandboxGate{launch: cfg.Sandbox, owner: owner, dispatched: true}
	receipt := gate.receipt()
	require.NotNil(t, receipt, "the completed request must contain its detached children before issuing a receipt")
	require.NoError(t, receipt.ValidateStopped(cfg.Sandbox))
	empty, err = cfg.Sandbox.Containment.Empty()
	require.NoError(t, err)
	require.True(t, empty)
}
