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
	case <-time.After(2 * time.Second):
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
	if result.JobID != "" || result.Operation != string(managedruntime.OperationUpToDate) {
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
