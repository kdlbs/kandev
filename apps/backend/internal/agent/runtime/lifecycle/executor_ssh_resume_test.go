package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestSSHBuildResumedInstanceRecordsProcessReuse(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	executor := &SSHExecutor{logger: log}
	for _, tc := range []struct {
		name           string
		reusingProcess bool
	}{
		{name: "running agent", reusingProcess: true},
		{name: "stopped agent", reusingProcess: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance := executor.buildResumedInstance(&ExecutorCreateRequest{
				InstanceID: "instance-1",
				TaskID:     "task-1",
				SessionID:  "session-1",
				Metadata: map[string]interface{}{
					MetadataKeySSHRemoteAgentctlPort: "43123",
					MetadataKeySSHRemoteTaskDir:      "/remote/task",
				},
			}, &sshSessionState{
				target:         &SSHTarget{Host: "ssh.example", Port: 22, User: "agent"},
				forwarder:      &SSHPortForwarder{localPort: 43124},
				pid:            99,
				remoteDir:      "/remote/session",
				authToken:      "resume-token",
				reusingProcess: tc.reusingProcess,
			})

			reusingProcess, ok := instance.Metadata["reuse_existing_process"].(bool)
			if !ok || reusingProcess != tc.reusingProcess {
				t.Fatalf("reuse_existing_process = (%v, %v), want (%v, true)", reusingProcess, ok, tc.reusingProcess)
			}
		})
	}
}

func TestSSHResumedAgentctlClientChecksExistingProcess(t *testing.T) {
	for _, tc := range []struct {
		name        string
		statusCode  int
		response    string
		wantReusing bool
	}{
		{name: "running", statusCode: http.StatusOK, response: `{"agent_status":"running"}`, wantReusing: true},
		{name: "starting", statusCode: http.StatusOK, response: `{"agent_status":"starting"}`, wantReusing: true},
		{name: "stopped", statusCode: http.StatusOK, response: `{"agent_status":"stopped"}`},
		{name: "status error", statusCode: http.StatusInternalServerError, response: "agentctl unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const previousInstanceID = "previous-instance"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("X-Instance-ID"); got != previousInstanceID {
					t.Errorf("X-Instance-ID = %q, want %q", got, previousInstanceID)
					http.Error(w, "stale instance client", http.StatusConflict)
					return
				}
				if got := r.Header.Get("Authorization"); got != "Bearer resume-token" {
					t.Errorf("Authorization = %q, want authenticated request", got)
				}
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.URL.Path != "/api/v1/status" {
					t.Errorf("path = %q, want /api/v1/status or /health", r.URL.Path)
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.response))
			}))
			t.Cleanup(server.Close)

			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("parse test server URL: %v", err)
			}
			port, err := strconv.Atoi(serverURL.Port())
			if err != nil {
				t.Fatalf("parse test server port: %v", err)
			}
			log, err := logger.NewFromZap(zap.NewNop())
			if err != nil {
				t.Fatalf("new logger: %v", err)
			}
			executor := &SSHExecutor{logger: log}

			req := &ExecutorCreateRequest{
				InstanceID:          "current-instance",
				PreviousExecutionID: previousInstanceID,
				SessionID:           "session-1",
				AuthToken:           "resume-token",
				Metadata: map[string]interface{}{
					MetadataKeySSHRemoteAgentctlPort: "43123",
					MetadataKeySSHRemoteTaskDir:      "/remote/task",
				},
			}
			client, reusingProcess := executor.newResumedAgentctlClient(context.Background(), port, req)
			if client == nil {
				t.Fatal("expected agentctl client")
			}
			if reusingProcess != tc.wantReusing {
				t.Fatalf("reusing process = %v, want %v", reusingProcess, tc.wantReusing)
			}
			instance := executor.buildResumedInstance(req, &sshSessionState{
				target:         &SSHTarget{Host: "ssh.example", Port: 22, User: "agent"},
				forwarder:      &SSHPortForwarder{localPort: port},
				agentctlClient: client,
				pid:            99,
				remoteDir:      "/remote/session",
				authToken:      req.AuthToken,
			})
			if instance.InstanceID != req.InstanceID {
				t.Fatalf("resumed instance ID = %q, want current ID %q", instance.InstanceID, req.InstanceID)
			}
			if instance.Client != client {
				t.Fatal("resumed instance did not retain the status-probed client")
			}
			if err := instance.Client.Health(context.Background()); err != nil {
				t.Fatalf("retained resumed client health: %v", err)
			}
		})
	}
}

func TestResumedSSHAgentctlInstanceID(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous string
		want     string
	}{
		{name: "previous execution", previous: "previous-instance", want: "previous-instance"},
		{name: "no previous execution", want: "current-instance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resumedSSHAgentctlInstanceID(&ExecutorCreateRequest{
				InstanceID:          "current-instance",
				PreviousExecutionID: tc.previous,
			})
			if got != tc.want {
				t.Fatalf("resumed SSH agentctl instance ID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSSHStopInstanceKillsRemoteAgentctlOnlyWhenCleanupIsRequired(t *testing.T) {
	tests := []struct {
		name     string
		force    bool
		instance *ExecutorInstance
		want     bool
	}{
		{
			name: "backend shutdown preserves resumable agentctl",
			instance: &ExecutorInstance{
				StopReason: StopReasonBackendShutdown,
			},
		},
		{
			name:     "non-shutdown stop kills agentctl",
			instance: &ExecutorInstance{},
			want:     true,
		},
		{
			name:  "force stop kills agentctl",
			force: true,
			instance: &ExecutorInstance{
				StopReason: StopReasonBackendShutdown,
			},
			want: true,
		},
		{
			name: "failed agent stop kills agentctl",
			instance: &ExecutorInstance{
				StopReason:      StopReasonBackendShutdown,
				AgentStopFailed: true,
			},
			want: true,
		},
		{
			name: "terminal task deletion kills agentctl",
			instance: &ExecutorInstance{
				StopReason: StopReasonTaskDeleted,
			},
			want: true,
		},
		{
			name: "stale replacement cleanup kills agentctl",
			instance: &ExecutorInstance{
				StopReason: stopReasonStaleExecutionCleanup,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sshShouldStopRemoteAgentctl(tt.instance, tt.force); got != tt.want {
				t.Fatalf("sshShouldStopRemoteAgentctl() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSSHRemoteCleanupContextIgnoresCanceledParentAndIsBounded(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	parentCancel()

	cleanupCtx, cleanupCancel := sshRemoteCleanupContext(parent)
	defer cleanupCancel()

	if err := cleanupCtx.Err(); err != nil {
		t.Fatalf("cleanup context should remain usable after parent cancellation, got %v", err)
	}
	if _, ok := cleanupCtx.Deadline(); !ok {
		t.Fatal("cleanup context should be bounded")
	}
}
