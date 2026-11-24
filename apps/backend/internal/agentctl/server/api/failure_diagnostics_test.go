package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	diagnosticTestTaskID     = "11111111-1111-4111-8111-111111111111"
	diagnosticTestSessionID  = "22222222-2222-4222-8222-222222222222"
	diagnosticTestInstanceID = "33333333-3333-4333-8333-333333333333"
)

func newDiagnosticTestServer(t *testing.T, cfg *config.InstanceConfig) (*Server, *observer.ObservedLogs) {
	t.Helper()
	return newDiagnosticTestServerAt(t, cfg, zapcore.ErrorLevel)
}

func newDiagnosticTestServerAt(
	t *testing.T,
	cfg *config.InstanceConfig,
	level zapcore.Level,
) (*Server, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(level)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		if tracker := manager.GetWorkspaceTracker(); tracker != nil {
			tracker.Stop()
		}
	})
	return NewServer(cfg, manager, nil, nil, log), observed
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestAgentConfigureFailureDiagnostic(t *testing.T) {
	t.Run("running refusal keeps its response and adds trusted identity", func(t *testing.T) {
		cfg := &config.InstanceConfig{
			WorkDir:    t.TempDir(),
			TaskID:     diagnosticTestTaskID,
			SessionID:  diagnosticTestSessionID,
			InstanceID: diagnosticTestInstanceID,
		}
		server, observed := newDiagnosticTestServer(t, cfg)
		server.procMgr.SetStatusForTest(process.StatusRunning)

		body := `{"command":"legacy command","agent_args":["runner","two words"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		var response AgentConfigureResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		require.False(t, response.Success)
		require.Equal(t, "cannot configure while agent is running", response.Error)

		entries := observed.FilterMessage("failed to configure agent").All()
		require.Len(t, entries, 1)
		fields := entries[0].ContextMap()
		require.Equal(t, "agent_configure", fields["operation"])
		require.Equal(t, "agent_running", fields["error_class"])
		require.Equal(t, diagnosticTestTaskID, fields["task_id"])
		require.Equal(t, diagnosticTestSessionID, fields["session_id"])
		require.Equal(t, diagnosticTestInstanceID, fields["instance_id"])
	})

	t.Run("missing and malformed identity is omitted", func(t *testing.T) {
		cfg := &config.InstanceConfig{
			WorkDir:   t.TempDir(),
			TaskID:    "not-a-uuid",
			SessionID: strings.ToUpper("abcdefab-abcd-4abc-8abc-abcdefabcdef"),
		}
		server, observed := newDiagnosticTestServer(t, cfg)
		server.procMgr.SetStatusForTest(process.StatusStarting)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", strings.NewReader(`{"command":"runner"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)

		entries := observed.FilterMessage("failed to configure agent").All()
		require.Len(t, entries, 1)
		fields := entries[0].ContextMap()
		require.Equal(t, "agent_running", fields["error_class"])
		for _, key := range []string{"task_id", "session_id", "instance_id"} {
			require.NotContains(t, fields, key)
		}
	})
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestGitStatusFailureDiagnostic(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	runGitAPI(t, repo, "init", "--initial-branch=main")
	runGitAPI(t, repo, "config", "user.email", "test@test.com")
	runGitAPI(t, repo, "config", "user.name", "Test User")
	writeFileAPI(t, repo, "README.md", "base\n")
	runGitAPI(t, repo, "add", ".")
	runGitAPI(t, repo, "commit", "-m", "initial")

	cfg := &config.InstanceConfig{
		WorkDir:   repo,
		TaskID:    diagnosticTestTaskID,
		SessionID: diagnosticTestSessionID,
	}
	server, observed := newDiagnosticTestServer(t, cfg)

	// Remove the repository context after the tracker resolved it, so a fresh
	// status runs Git in a directory that is no longer a repository.
	require.NoError(t, os.Rename(filepath.Join(repo, ".git"), filepath.Join(repo, ".git-unavailable")))

	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/git/status?fresh=true", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var result GitStatusResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.False(t, result.Success)
	require.Equal(t, "unavailable", result.StatusState)
	require.NotEmpty(t, result.ErrorCode)

	entries := observed.FilterMessage("git status failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "git_inspection", fields["operation"])
	require.Equal(t, "repository_context_unavailable", fields["error_class"])
	require.Equal(t, diagnosticTestTaskID, fields["task_id"])
	require.Equal(t, diagnosticTestSessionID, fields["session_id"])

	// The multi-repository fan-out shares the same bounded failure record.
	multiRec := httptest.NewRecorder()
	server.Router().ServeHTTP(multiRec,
		httptest.NewRequest(http.MethodGet, "/api/v1/git/status/multi?fresh=true", nil))
	require.Equal(t, http.StatusOK, multiRec.Code)
	var multiResult MultiRepoGitStatusResult
	require.NoError(t, json.Unmarshal(multiRec.Body.Bytes(), &multiResult))
	require.Len(t, multiResult.Repos, 1)
	require.False(t, multiResult.Repos[0].Status.Success)

	multiEntries := observed.FilterMessage("git status failed").All()
	require.Len(t, multiEntries, 2, "the multi-repository path keeps the same failure attribution")
	require.Equal(t, "git_inspection", multiEntries[1].ContextMap()["operation"])
	require.Equal(t, "repository_context_unavailable", multiEntries[1].ContextMap()["error_class"])
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9
func TestGitStatusEvidenceChangeDiagnostic(t *testing.T) {
	cfg := &config.InstanceConfig{
		WorkDir:   t.TempDir(),
		TaskID:    diagnosticTestTaskID,
		SessionID: diagnosticTestSessionID,
	}
	server, observed := newDiagnosticTestServerAt(t, cfg, zapcore.WarnLevel)

	server.logGitStatusFailure(process.GitStatusEvidenceChangedErrorForTest())

	entries := observed.FilterMessage("git status capture failed due to changing repository evidence").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "git_inspection", fields["operation"])
	require.Equal(t, "evidence_changed", fields["error_class"])
	require.Equal(t, diagnosticTestTaskID, fields["task_id"])
	require.Equal(t, diagnosticTestSessionID, fields["session_id"])
}
