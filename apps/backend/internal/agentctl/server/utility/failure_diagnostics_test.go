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
	handler := NewHandler("", zap.New(core))
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1"))

	body, err := json.Marshal(ProbeRequest{
		AgentID:        "managed-acp",
		ProfileContext: true,
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3"},
			WorkDir: t.TempDir(),
			Env:     map[string]string{"HOME": t.TempDir()},
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inference/probe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

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
	require.Equal(t, response.Error, fields[inferenceErrorKey])
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
			for _, field := range probeExecutionFailureFields(test.err) {
				fields[field.Key] = field.String
			}
			require.Equal(t, probeDiagnosticOperation, fields["operation"])
			require.Equal(t, test.want, fields["error_class"])
		})
	}
}
