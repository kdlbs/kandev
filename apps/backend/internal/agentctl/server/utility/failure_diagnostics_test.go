package utility

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const probeDiagnosticTestInstanceID = "33333333-3333-4333-8333-333333333333"

// failingProbeExecutor lets handler tests reach the execution-error branch
// without a real subprocess.
type failingProbeExecutor struct{ err error }

func (f failingProbeExecutor) Probe(context.Context, *ProbeRequest) (*ProbeResponse, error) {
	return nil, f.err
}

func (f failingProbeExecutor) Execute(context.Context, *PromptRequest) (*PromptResponse, error) {
	return nil, f.err
}

func newProbeDiagnosticRouter(t *testing.T, handler *Handler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1"))
	return router
}

func postProbeRequest(t *testing.T, router *gin.Engine, request ProbeRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inference/probe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestProbeFailureSummaryDiagnostic(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fixture uses a POSIX shell script")
	}
	binDir := t.TempDir()
	writeProfileProbeFixture(t, filepath.Join(binDir, "npx"),
		"#!/bin/sh\nprintf 'authentication required\\n' >&2\nexit 1\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	core, observed := observer.New(zapcore.WarnLevel)
	handler := NewHandler("", nil, zap.New(core))
	handler.SetInstanceID(probeDiagnosticTestInstanceID)
	router := newProbeDiagnosticRouter(t, handler)

	rec := postProbeRequest(t, router, ProbeRequest{
		AgentID:        "managed-acp",
		ProfileContext: true,
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3"},
			WorkDir: t.TempDir(),
			Env:     map[string]string{"HOME": t.TempDir()},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)
	var response ProbeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Equal(t, ProbeFailureAuthenticationRequired, response.FailureCode)
	require.Equal(t, "agent authentication is required", response.Error)

	entries := observed.FilterMessage("ACP probe unsuccessful").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, probeDiagnosticOperation, fields["operation"])
	require.Equal(t, string(ProbeFailureAuthenticationRequired), fields["error_class"])
	require.Equal(t, probeDiagnosticTestInstanceID, fields["instance_id"])
	require.Equal(t, response.Error, fields[inferenceErrorKey])
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestProbeExecutionFailureHandlerDiagnostic(t *testing.T) {
	core, observed := observer.New(zapcore.ErrorLevel)
	handler := NewHandler("", nil, zap.New(core))
	handler.SetInstanceID(probeDiagnosticTestInstanceID)
	handler.acpExecutor = failingProbeExecutor{err: context.DeadlineExceeded}
	router := newProbeDiagnosticRouter(t, handler)

	rec := postProbeRequest(t, router, ProbeRequest{AgentID: "mock-agent"})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var response ProbeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Equal(t, "probe execution failed", response.Error)

	entries := observed.FilterMessage("inference probe failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, probeDiagnosticOperation, fields["operation"])
	require.Equal(t, "deadline_exceeded", fields["error_class"])
	require.Equal(t, probeDiagnosticTestInstanceID, fields["instance_id"])
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestSetInstanceIDRejectsInvalidIdentity(t *testing.T) {
	handler := NewHandler("", nil, zap.NewNop())
	for _, invalid := range []string{"", "not-a-uuid", "33333333-3333-4333-8333-33333333333A"} {
		handler.SetInstanceID(invalid)
		require.Empty(t, handler.instanceID)
	}
	handler.SetInstanceID(probeDiagnosticTestInstanceID)
	require.Equal(t, probeDiagnosticTestInstanceID, handler.instanceID)
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestProbeFailureCodeClass(t *testing.T) {
	for _, code := range []ProbeFailureCode{
		ProbeFailureManagedRuntimeNPMResolution,
		ProbeFailureManagedRuntimeNPMPolicy,
		ProbeFailureUnsupportedContext,
		ProbeFailureAuthenticationRequired,
	} {
		require.Equal(t, string(code), probeFailureCodeClass(code))
	}
	for _, code := range []ProbeFailureCode{"", "free_form_provider_error", "agent_running"} {
		require.Equal(t, probeDiagnosticUnknown, probeFailureCodeClass(code))
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestProbeExecutionFailureDiagnostic(t *testing.T) {
	handler := NewHandler("", nil, zap.NewNop())
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: "deadline_exceeded"},
		{name: "cancellation", err: context.Canceled, want: "context_canceled"},
		{name: "unclassified", err: errors.New("provider exploded"), want: probeDiagnosticUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields := map[string]any{}
			for _, field := range handler.probeExecutionFailureFields(test.err) {
				fields[field.Key] = field.String
			}
			require.Equal(t, probeDiagnosticOperation, fields["operation"])
			require.Equal(t, test.want, fields["error_class"])
			require.NotContains(t, fields, "instance_id", "identity stays omitted when unavailable")
		})
	}
}
