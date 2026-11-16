package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type handlerRuntimeUpdater struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type handlerSelectionStore struct {
	selection managedruntime.Selection
}

func (s handlerSelectionStore) Get(
	context.Context,
	string,
	string,
) (managedruntime.Selection, bool, error) {
	return s.selection, true, nil
}

func (handlerSelectionStore) Save(context.Context, string, string, string) error {
	return nil
}

func (handlerSelectionStore) Delete(context.Context, string, string) error {
	return nil
}

func (u *handlerRuntimeUpdater) CurrentCapabilities(string) (hostutility.AgentCapabilities, bool) {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}, true
}

func (u *handlerRuntimeUpdater) ResolveTarget(context.Context, string) (string, error) {
	return "1.1.0", nil
}

type reviewedHandlerRuntimeUpdater struct {
	handlerRuntimeUpdater
}

func (u *reviewedHandlerRuntimeUpdater) ResolveVersions(context.Context, string) (controller.RuntimeVersionMetadata, error) {
	return controller.RuntimeVersionMetadata{Versions: []string{"1.0.0", "1.1.0"}, Latest: "1.1.0"}, nil
}

func (u *reviewedHandlerRuntimeUpdater) Probe(
	context.Context,
	string,
	agents.Command,
) (hostutility.AgentCapabilities, error) {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}, nil
}

func (u *reviewedHandlerRuntimeUpdater) PublishCapabilities(string, hostutility.AgentCapabilities) {}

func (u *handlerRuntimeUpdater) RunUpdate(
	ctx context.Context,
	_ agents.Command,
	onChunk func(string),
) error {
	if u.started != nil {
		u.once.Do(func() { close(u.started) })
	}
	if u.release != nil {
		select {
		case <-u.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	onChunk("updated\n")
	return nil
}

func (u *handlerRuntimeUpdater) InvalidateExecutionCache(context.Context, string) error {
	return nil
}

func (u *handlerRuntimeUpdater) Refresh(
	context.Context,
	string,
	agents.Command,
) (hostutility.AgentCapabilities, error) {
	return hostutility.AgentCapabilities{
		Status:       hostutility.StatusOK,
		AgentVersion: "1.1.0",
	}, nil
}

type updateTerminalBroadcaster struct {
	completed chan dto.AgentUpdateJobDTO
}

func newUpdateTerminalBroadcaster() *updateTerminalBroadcaster {
	return &updateTerminalBroadcaster{completed: make(chan dto.AgentUpdateJobDTO, 2)}
}

func (b *updateTerminalBroadcaster) Broadcast(message *ws.Message) {
	if message.Action != ws.ActionAgentUpdateFinished {
		return
	}
	var job dto.AgentUpdateJobDTO
	if json.Unmarshal(message.Payload, &job) != nil {
		return
	}
	select {
	case b.completed <- job:
	default:
	}
}

func waitForTerminalUpdate(
	t *testing.T,
	completed <-chan dto.AgentUpdateJobDTO,
	jobID string,
) dto.AgentUpdateJobDTO {
	t.Helper()
	select {
	case job := <-completed:
		if job.JobID != jobID {
			t.Fatalf("finished job = %s, want %s", job.JobID, jobID)
		}
		return job
	case <-time.After(10 * time.Second):
		t.Fatalf("job %s did not finish in time", jobID)
		return dto.AgentUpdateJobDTO{}
	}
}

func newAgentUpdateRouter(
	t *testing.T,
	updater controller.RuntimeUpdater,
) (*gin.Engine, *controller.Controller, <-chan dto.AgentUpdateJobDTO) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), reg, log)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	ctrl := controller.NewController(nil, discoveryRegistry, reg, nil, log)
	ctrl.SetRuntimeUpdater(updater)
	router := gin.New()
	useSyntheticSettingsIdentity(router)
	broadcaster := newUpdateTerminalBroadcaster()
	RegisterRoutes(router, ctrl, broadcaster, log, "test-interlock")
	return router, ctrl, broadcaster.completed
}

func updateRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	return request
}

func updateJSONRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	return request
}

func TestAgentUpdatePreviewEndpointIsReadOnly(t *testing.T) {
	router, _, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/claude-acp/preview",
	))
	if response.Code != http.StatusOK {
		t.Fatalf("GET preview status = %d, body = %s", response.Code, response.Body.String())
	}
	var preview map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview["current_version"] != "1.0.0" || preview["target_version"] != "1.1.0" {
		t.Fatalf("versions = %#v", preview)
	}
	commandArgs := agents.NewClaudeACP().ManagedNPMRuntime().CacheUpdateCommand().Args()
	wantCommand := strings.Join(commandArgs[:len(commandArgs)-1], " ") + ` ""`
	if preview["command_string"] != wantCommand {
		t.Fatalf("command_string = %#v", preview["command_string"])
	}

	jobs := httptest.NewRecorder()
	router.ServeHTTP(jobs, updateRequest(http.MethodGet, "/api/v1/agent-update/jobs"))
	if jobs.Code != http.StatusOK || !strings.Contains(jobs.Body.String(), `"jobs":[]`) {
		t.Fatalf("preview must not create a job: %d %s", jobs.Code, jobs.Body.String())
	}
}

func TestAgentUpdateStatusEndpointIsReadOnly(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	router, ctrl, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})
	var statusResponse struct {
		Statuses []dto.AgentUpdateStatusDTO `json:"statuses"`
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateRequest(http.MethodGet, "/api/v1/agent-update/status"))
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &statusResponse); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if len(statusResponse.Statuses) == 0 {
		t.Fatal("status response is empty")
	}
	if jobs := ctrl.ListAgentUpdateJobs(); len(jobs) != 0 {
		t.Fatalf("status request created %d update jobs", len(jobs))
	}
}

func TestAgentUpdateEndpointsAcceptAndRetainJobs(t *testing.T) {
	router, _, completed := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateJSONRequest(
		http.MethodPost,
		"/api/v1/agent-update/claude-acp",
		`{"target_version":"1.1.0"}`,
	))
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, body = %s", response.Code, response.Body.String())
	}
	var accepted dto.AgentUpdateJobDTO
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted job: %v", err)
	}

	finished := waitForTerminalUpdate(t, completed, accepted.JobID)
	if finished.Status != dto.AgentUpdateJobStatusSucceeded {
		t.Fatalf("finished status = %q, want succeeded", finished.Status)
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/jobs/"+accepted.JobID,
	))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET job status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var retained dto.AgentUpdateJobDTO
	if err := json.Unmarshal(getResponse.Body.Bytes(), &retained); err != nil {
		t.Fatalf("decode retained job: %v", err)
	}
	if retained.Status != dto.AgentUpdateJobStatusSucceeded {
		t.Fatalf("retained status = %q, want succeeded", retained.Status)
	}

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, updateRequest(http.MethodGet, "/api/v1/agent-update/jobs"))
	if listResponse.Code != http.StatusOK ||
		!strings.Contains(listResponse.Body.String(), accepted.JobID) {
		t.Fatalf("list response = %d %s", listResponse.Code, listResponse.Body.String())
	}
}

func TestAgentUpdateEndpointDoesNotCreateAlreadyActiveHealthyJob(t *testing.T) {
	router, ctrl, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})
	selectionStore := handlerSelectionStore{selection: managedruntime.Selection{
		Package: "@agentclientprotocol/claude-agent-acp",
		Version: "1.0.0",
	}}
	ctrl.SetManagedRuntimeSelectionStore(selectionStore)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateJSONRequest(
		http.MethodPost,
		"/api/v1/agent-update/claude-acp",
		`{"target_version":"1.0.0"}`,
	))
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, body = %s", response.Code, response.Body.String())
	}
	var result dto.AgentUpdateJobDTO
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode no-op response: %v", err)
	}
	if result.JobID != "" || result.Operation != string(managedruntime.OperationUpToDate) || result.UpdateMode != dto.AgentUpdateModePinned {
		t.Fatalf("no-op response = %#v, want terminal up_to_date without job ID", result)
	}

	jobs := httptest.NewRecorder()
	router.ServeHTTP(jobs, updateRequest(http.MethodGet, "/api/v1/agent-update/jobs"))
	if jobs.Code != http.StatusOK || !strings.Contains(jobs.Body.String(), `"jobs":[]`) {
		t.Fatalf("no-op jobs response = %d %s", jobs.Code, jobs.Body.String())
	}
}

func TestAgentUpdateEndpointReturnsMaintenanceConflict(t *testing.T) {
	updater := &handlerRuntimeUpdater{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	router, _, completed := newAgentUpdateRouter(t, updater)
	t.Cleanup(func() {
		select {
		case <-updater.release:
		default:
			close(updater.release)
		}
	})
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, updateJSONRequest(
		http.MethodPost,
		"/api/v1/agent-update/claude-acp",
		`{"target_version":"1.1.0"}`,
	))
	if updateResponse.Code != http.StatusAccepted {
		t.Fatalf("update status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
	<-updater.started

	installResponse := httptest.NewRecorder()
	router.ServeHTTP(installResponse, updateRequest(
		http.MethodPost,
		"/api/v1/agent-install/claude-acp",
	))
	if installResponse.Code != http.StatusConflict {
		t.Fatalf("install status = %d, body = %s", installResponse.Code, installResponse.Body.String())
	}
	if !strings.Contains(installResponse.Body.String(), `"active_kind":"update"`) {
		t.Fatalf("conflict body = %s", installResponse.Body.String())
	}
	close(updater.release)

	var accepted dto.AgentUpdateJobDTO
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted update: %v", err)
	}
	finished := waitForTerminalUpdate(t, completed, accepted.JobID)
	if finished.Status != dto.AgentUpdateJobStatusSucceeded {
		t.Fatalf("finished status = %q, want succeeded", finished.Status)
	}
}

func TestAgentUpdateEndpointRejectsUnmanagedAgentAndMissingJob(t *testing.T) {
	router, _, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})

	unsupported := httptest.NewRecorder()
	router.ServeHTTP(unsupported, updateRequest(
		http.MethodPost,
		"/api/v1/agent-update/cursor-acp",
	))
	if unsupported.Code != http.StatusBadRequest {
		t.Fatalf("unsupported status = %d, body = %s", unsupported.Code, unsupported.Body.String())
	}

	unsupportedPreview := httptest.NewRecorder()
	router.ServeHTTP(unsupportedPreview, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/cursor-acp/preview",
	))
	if unsupportedPreview.Code != http.StatusBadRequest {
		t.Fatalf("unsupported preview status = %d, body = %s", unsupportedPreview.Code, unsupportedPreview.Body.String())
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/jobs/missing",
	))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, body = %s", missing.Code, missing.Body.String())
	}
}

func TestAgentUpdateEndpointRequiresExactTargetVersion(t *testing.T) {
	router, _, _ := newAgentUpdateRouter(t, &handlerRuntimeUpdater{})
	for _, body := range []string{"", `{}`, `{"target_version":"latest"}`} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, updateJSONRequest(
			http.MethodPost,
			"/api/v1/agent-update/claude-acp",
			body,
		))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d, want 400: %s", body, response.Code, response.Body.String())
		}
	}
}

type handlerHarnessUpdater struct {
	handlerRuntimeUpdater
	current string
}

func (u *handlerHarnessUpdater) CurrentCapabilities(string) (hostutility.AgentCapabilities, bool) {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: u.current}, true
}

func (u *handlerHarnessUpdater) ResolveHarnessLatest(context.Context, string) (string, error) {
	return "1.1.0", nil
}

// A stable reference does not suppress the harness updater; target fields remain rejected.
func TestHarnessUpdateHTTPApprovalAndRejectedTargets(t *testing.T) {
	router, ctrl, completed := newAgentUpdateRouter(t, &handlerHarnessUpdater{current: "1.1.0"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateJSONRequest(http.MethodPost, "/api/v1/agent-update/omp-acp", `{}`))
	if response.Code != http.StatusAccepted {
		t.Fatalf("approval status %d: %s", response.Code, response.Body.String())
	}
	var job dto.AgentUpdateJobDTO
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.JobID == "" || job.UpdateMode != dto.AgentUpdateModeSelfUpdate {
		t.Fatalf("stable version suppressed update job: %+v", job)
	}
	finished := waitForTerminalUpdate(t, completed, job.JobID)
	if finished.Status != dto.AgentUpdateJobStatusFailed || !strings.Contains(finished.Error, "probe unavailable") {
		t.Errorf("harness without candidate probe result = %+v", finished)
	}
	jobCount := len(ctrl.ListAgentUpdateJobs())
	if jobCount != 1 {
		t.Fatalf("retained jobs = %d, want accepted job", jobCount)
	}
	for _, body := range []string{`{"target_version":"1.2.0"}`, `{"use_default":true}`, `{"command":["sh","-c","exit 0"],"target_version":"1.2.0"}`} {
		rejected := httptest.NewRecorder()
		router.ServeHTTP(rejected, updateJSONRequest(http.MethodPost, "/api/v1/agent-update/omp-acp", body))
		if rejected.Code != http.StatusBadRequest {
			t.Errorf("body %s status = %d: %s", body, rejected.Code, rejected.Body.String())
		}
		if jobs := ctrl.ListAgentUpdateJobs(); len(jobs) != jobCount {
			t.Errorf("rejected body %s changed retained jobs to %d", body, len(jobs))
		}
	}
}

func TestAgentUpdateReviewedRuntimeRoundTrip(t *testing.T) {
	router, ctrl, completed := newAgentUpdateRouter(t, &reviewedHandlerRuntimeUpdater{})
	ctrl.SetManagedRuntimeSelectionStore(handlerSelectionStore{})
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/claude-acp/preview?target_version=1.1.0",
	))
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview dto.AgentUpdatePreviewDTO
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.ReviewedRuntime == nil {
		t.Fatal("managed preview did not issue reviewed runtime evidence")
	}
	body, err := json.Marshal(map[string]any{
		"target_version":   "1.1.0",
		"reviewed_runtime": preview.ReviewedRuntime,
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateJSONRequest(
		http.MethodPost, "/api/v1/agent-update/claude-acp", string(body),
	))
	if response.Code != http.StatusAccepted {
		t.Fatalf("reviewed POST status = %d, body = %s", response.Code, response.Body.String())
	}
	var accepted dto.AgentUpdateJobDTO
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted job: %v", err)
	}
	finished := waitForTerminalUpdate(t, completed, accepted.JobID)
	if finished.Status != dto.AgentUpdateJobStatusSucceeded {
		t.Fatalf("reviewed job status = %q, want succeeded", finished.Status)
	}
}

func TestAgentUpdateReviewedRuntimeRejectsStaleEvidence(t *testing.T) {
	router, ctrl, _ := newAgentUpdateRouter(t, &reviewedHandlerRuntimeUpdater{})
	ctrl.SetManagedRuntimeSelectionStore(handlerSelectionStore{})
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/claude-acp/preview?target_version=1.1.0",
	))
	var preview dto.AgentUpdatePreviewDTO
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	evidence := *preview.ReviewedRuntime
	evidence.CurrentVersion = "0.9.0"
	body, err := json.Marshal(map[string]any{
		"target_version":   "1.1.0",
		"reviewed_runtime": evidence,
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, updateJSONRequest(
		http.MethodPost, "/api/v1/agent-update/claude-acp", string(body),
	))
	if response.Code != http.StatusConflict {
		t.Fatalf("stale review status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"runtime_review_changed"`) {
		t.Fatalf("stale review body = %s", response.Body.String())
	}
}

func TestAgentUpdateReviewedRuntimeRejectsIncompatibleForms(t *testing.T) {
	router, ctrl, _ := newAgentUpdateRouter(t, &reviewedHandlerRuntimeUpdater{})
	ctrl.SetManagedRuntimeSelectionStore(handlerSelectionStore{})
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, updateRequest(
		http.MethodGet,
		"/api/v1/agent-update/claude-acp/preview?target_version=1.1.0",
	))
	var preview dto.AgentUpdatePreviewDTO
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	cases := []struct {
		name string
		body string
	}{
		{"use_default", `{"use_default":true,"reviewed_runtime":` + mustJSON(t, preview.ReviewedRuntime) + `}`},
		{"migration", `{"target_version":"1.1.0","target_family":"v2","expected_runtime_revision":1,"reviewed_runtime":` + mustJSON(t, preview.ReviewedRuntime) + `}`},
		{"incomplete", `{"target_version":"1.1.0","reviewed_runtime":{"runtime_id":"","package":"","current_version":"","effective_version":""}}`},
		{"legacy_still_valid", `{"target_version":"1.1.0"}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, updateJSONRequest(
				http.MethodPost, "/api/v1/agent-update/claude-acp", testCase.body,
			))
			if testCase.name == "legacy_still_valid" {
				if response.Code != http.StatusAccepted {
					t.Fatalf("legacy POST status = %d, body = %s", response.Code, response.Body.String())
				}
				return
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("incompatible review status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return string(encoded)
}
