package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestSSHRecoveryReattachesRecordedRemoteProcessWithoutStartingWork(t *testing.T) {
	controlServer := newSSHRecoveryInstanceInfoServer(t, agentctl.InstanceInfo{
		ID: "native-execution-id", Port: 41234, Status: "running", WorkspacePath: "/remote/task",
		SessionID: "session-ssh", TaskID: "task-ssh", Env: map[string]string{"SSH_LIVE": "current"},
		WorkspaceSourceRoots: []string{"/remote/task/src"},
	})
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "ps -p 4242 -o command="):
			return sshExecResult{Stdout: "/opt/kandev/bin/agentctl --workdir /remote/task"}
		case command == "cat -- '/remote/session/agentctl.pid'":
			return sshExecResult{Stdout: "4242"}
		default:
			return sshExecResult{Stderr: "unexpected command", ExitCode: 127}
		}
	})
	server.forwardTo(controlServer.Listener.Addr().String())
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteControlPort] = "41235"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	metadata[MetadataKeySSHAgentctlInstanceID] = "native-execution-id"
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SSH recovery is not wired to detailed candidate outcomes")
	}
	recoveryCtx, cancelRecovery := context.WithCancel(context.Background())
	instances, outcomes, err := recovery.RecoverInstancesDetailed(recoveryCtx, []*models.ExecutorRunning{{
		ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh",
		AgentExecutionID: "backend-execution-id", Runtime: agentruntime.RuntimeSSH,
		PID: 4242, TransientAuthToken: "refreshed-auth-token", Metadata: metadata,
	}})
	cancelRecovery()
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("recovered instances = %d, want one attached saved SSH process", len(instances))
	}
	instance := instances[0]
	if instance.InstanceID != "backend-execution-id" || instance.SessionID != "session-ssh" || instance.TaskID != "task-ssh" {
		t.Fatalf("recovered identity = instance=%q session=%q task=%q", instance.InstanceID, instance.SessionID, instance.TaskID)
	}
	if instance.Client == nil || instance.Client.AuthToken() != "refreshed-auth-token" {
		t.Fatal("SSH recovery did not refresh the authenticated agentctl transport")
	}
	exec.mu.Lock()
	state := exec.sessions["backend-execution-id"]
	exec.mu.Unlock()
	if state == nil || state.client == nil || state.controlPort != 41235 {
		t.Fatal("recovery transport was not transferred to the SSH executor")
	}
	if ok, _, err := state.client.SendRequest("keepalive@openssh.com", true, nil); err != nil || ok {
		t.Fatalf("saved SSH transport after recovery context cancellation = (%v, %v), want connected transport", ok, err)
	}
	if getMetadataString(instance.Metadata, MetadataKeySSHAgentctlInstanceID) != "native-execution-id" {
		t.Fatalf("recovered native identity = %q, want persisted native process identity", getMetadataString(instance.Metadata, MetadataKeySSHAgentctlInstanceID))
	}
	if got := getMetadataString(instance.Metadata, MetadataKeySSHRemoteControlPort); got != "41235" {
		t.Fatalf("recovered control listener port = %q, want the exact live listener", got)
	}
	if getMetadataString(instance.Metadata, MetadataKeySSHRemoteSessionDir) != "/remote/session" || instance.WorkspacePath != "/remote/task" {
		t.Fatalf("recovered remote paths = metadata %v workspace %q", instance.Metadata, instance.WorkspacePath)
	}
	if instance.Env["SSH_LIVE"] != "current" || len(instance.WorkspaceSourceRoots) != 1 || instance.WorkspaceSourceRoots[0] != "/remote/task/src" {
		t.Fatalf("recovered live environment/source roots = %#v / %#v", instance.Env, instance.WorkspaceSourceRoots)
	}
	duplicate, duplicateOutcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh",
		AgentExecutionID: "backend-execution-id", Runtime: agentruntime.RuntimeSSH,
		PID: 4242, TransientAuthToken: "refreshed-auth-token", Metadata: metadata,
	}})
	if err != nil {
		t.Fatalf("duplicate RecoverInstancesDetailed: %v", err)
	}
	if len(duplicate) != 0 || duplicateOutcomes["session-ssh"] != RecoveryOutcomeUnknown {
		t.Fatalf("duplicate recovery returned %d stale instances, outcome %q; want retryable unknown", len(duplicate), duplicateOutcomes["session-ssh"])
	}
	instance.DiscardRecovery()
	instance.Client.Close()
	exec.mu.Lock()
	removedCandidate := exec.sessions["backend-execution-id"] == nil
	exec.mu.Unlock()
	if !removedCandidate {
		t.Fatal("discarding a rejected recovery left its SSH state registered")
	}
	retried, retryOutcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh",
		AgentExecutionID: "backend-execution-id", Runtime: agentruntime.RuntimeSSH,
		PID: 4242, TransientAuthToken: "refreshed-auth-token", Metadata: metadata,
	}})
	if err != nil {
		t.Fatalf("retry RecoverInstancesDetailed: %v", err)
	}
	if len(retried) != 1 || retryOutcomes["session-ssh"] != "" || retried[0].Client == nil {
		t.Fatalf("retry after exact discard returned %d instances, outcome %q; want a new attached transport", len(retried), retryOutcomes["session-ssh"])
	}
	exec.mu.Lock()
	retriedState := exec.sessions["backend-execution-id"]
	exec.mu.Unlock()
	if retriedState == nil || retriedState == state {
		t.Fatal("retry did not establish a new SSH transport state")
	}
	if _, classified := outcomes["session-ssh"]; classified {
		t.Fatalf("successful recovery should be represented by its returned instance, got outcome %q", outcomes["session-ssh"])
	}
	if retried[0].DiscardRecovery == nil {
		t.Fatal("retried SSH instance lacks exact-attempt cleanup")
	}

	commands := server.commands()
	if len(commands) != 6 {
		t.Fatalf("recovery commands = %q, want three pairs of exact process and pidfile identity probes", commands)
	}
	for index := 0; index < len(commands); index += 2 {
		if !strings.Contains(commands[index], "ps -p 4242 -o command=") || commands[index+1] != "cat -- '/remote/session/agentctl.pid'" {
			t.Fatalf("recovery command pair = %q, want exact process and pidfile identity probes", commands[index:index+2])
		}
	}
	successor := &sshSessionState{}
	exec.mu.Lock()
	delete(exec.sessions, "backend-execution-id")
	exec.mu.Unlock()
	retried[0].DiscardRecovery()
	exec.mu.Lock()
	removedRetry := exec.sessions["backend-execution-id"] == nil
	exec.sessions["backend-execution-id"] = successor
	exec.mu.Unlock()
	if !removedRetry {
		t.Fatal("discarding retry did not remove its exact SSH state")
	}
	retried[0].DiscardRecovery()
	exec.mu.Lock()
	stillCurrent := exec.sessions["backend-execution-id"] == successor
	delete(exec.sessions, "backend-execution-id")
	exec.mu.Unlock()
	if !stillCurrent {
		t.Fatal("discarding a rejected recovery removed a successor SSH state")
	}
	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, command := range server.commands() {
		if strings.Contains(command, "kill ") || strings.Contains(command, "agentctl start") || strings.Contains(command, "mkdir") || strings.Contains(command, "prepare") {
			t.Fatalf("remote recovery/shutdown mutated compute with command %q", command)
		}
	}
}

func newSSHRecoveryInstanceInfoServer(t *testing.T, info agentctl.InstanceInfo) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/instances/"+info.ID {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer refreshed-auth-token" {
			http.Error(w, "missing control authentication", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(info); err != nil {
			t.Errorf("encode live instance info: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSSHRecoveryReadsLegacyControlPortFromExactSessionLog(t *testing.T) {
	controlServer := newSSHRecoveryInstanceInfoServer(t, agentctl.InstanceInfo{
		ID: "native-execution-id", Port: 41234, Status: "running", WorkspacePath: "/remote/task",
		SessionID: "session-ssh", TaskID: "task-ssh", Env: map[string]string{"SSH_LIVE": "legacy"},
	})
	legacyLog := `{"level":"info","msg":"HTTP server bound successfully","address":"127.0.0.1:41000"}` +
		"\n" + strings.Repeat("old output\n", 8192) +
		"\n" + `{"level":"info","msg":"HTTP server bound successfully","address":"127.0.0.1:41235"}` + "\n"
	boundedLog := legacyLog[:32768] + "\n" + legacyLog[len(legacyLog)-32768:]
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "ps -p 4242 -o command="):
			return sshExecResult{Stdout: "/opt/kandev/bin/agentctl --workdir /remote/task"}
		case command == "cat -- '/remote/session/agentctl.pid'":
			return sshExecResult{Stdout: "4242"}
		case command == "head -c 32768 '/remote/session/agentctl.log' 2>/dev/null; printf '\\n'; tail -c 32768 '/remote/session/agentctl.log' 2>/dev/null":
			return sshExecResult{Stdout: boundedLog}
		default:
			return sshExecResult{Stderr: "unexpected command", ExitCode: 127}
		}
	})
	server.forwardTo(controlServer.Listener.Addr().String())
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	metadata[MetadataKeySSHAgentctlInstanceID] = "native-execution-id"
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SSH recovery is not wired to detailed candidate outcomes")
	}
	instances, outcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh", AgentExecutionID: "backend-execution-id",
		Runtime: agentruntime.RuntimeSSH, PID: 4242, TransientAuthToken: "refreshed-auth-token", Metadata: metadata,
	}})
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 1 || outcomes["session-ssh"] != "" || instances[0].Env["SSH_LIVE"] != "legacy" {
		t.Fatalf("legacy SSH recovery = instances %d, outcome %q, env %#v", len(instances), outcomes["session-ssh"], recoveredSSHEnv(instances))
	}
	if got := getMetadataString(instances[0].Metadata, MetadataKeySSHRemoteControlPort); got != "41235" {
		t.Fatalf("legacy recovery control listener port = %q, want latest exact session bind", got)
	}
	instances[0].DiscardRecovery()
	instances[0].Client.Close()
	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	commands := server.commands()
	if len(commands) != 3 || commands[2] != "head -c 32768 '/remote/session/agentctl.log' 2>/dev/null; printf '\\n'; tail -c 32768 '/remote/session/agentctl.log' 2>/dev/null" {
		t.Fatalf("legacy control-port recovery commands = %q, want exact process proof then bounded head and tail", commands)
	}
}

func TestLatestSSHAgentctlControlPortRequiresLoopbackBind(t *testing.T) {
	for _, test := range []struct {
		name    string
		address string
		want    int
		wantErr bool
	}{
		{name: "json log", address: `{"level":"info","msg":"HTTP server bound successfully","address":"127.0.0.1:41001"}`, want: 41001},
		{name: "console log", address: `INFO HTTP server bound successfully address=127.0.0.1:41002`, want: 41002},
		{name: "ipv6 loopback", address: `{"msg":"HTTP server bound successfully","address":"[::1]:41003"}`, want: 41003},
		{name: "external interface", address: `{"msg":"HTTP server bound successfully","address":"10.0.0.3:41004"}`, wantErr: true},
		{name: "invalid latest bind", address: `{"msg":"HTTP server bound successfully","address":"bad-address"}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := latestSSHAgentctlControlPort(test.address)
			if test.wantErr {
				if err == nil {
					t.Fatalf("latestSSHAgentctlControlPort(%s) = %d, want error", test.address, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("latestSSHAgentctlControlPort(%s) = %d, %v, want %d", test.address, got, err, test.want)
			}
		})
	}
}

func recoveredSSHEnv(instances []*ExecutorInstance) map[string]string {
	if len(instances) != 1 || instances[0] == nil {
		return nil
	}
	return instances[0].Env
}

func TestSSHRecoveryKeepsUnprovenRemoteOwnerRetryable(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if strings.Contains(command, "ps -p 4242 -o command=") {
			return sshExecResult{Stderr: "remote ps unavailable", ExitCode: 1}
		}
		return sshExecResult{Stderr: "unexpected command", ExitCode: 127}
	})
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	metadata[MetadataKeySSHAgentctlInstanceID] = "native-execution-id"
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SSH recovery is not wired to detailed candidate outcomes")
	}
	instances, outcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh", AgentExecutionID: "execution-ssh",
		Runtime: agentruntime.RuntimeSSH, PID: 4242, TransientAuthToken: "auth-token", Metadata: metadata,
	}})
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 0 || outcomes["session-ssh"] != RecoveryOutcomeUnknown {
		t.Fatalf("unproven SSH owner result = instances %d, outcome %q; want retryable unknown", len(instances), outcomes["session-ssh"])
	}
	if got := server.commands(); len(got) != 1 || !strings.Contains(got[0], "ps -p 4242 -o command=") {
		t.Fatalf("unproven recovery commands = %q, want one read-only identity probe", got)
	}
	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSSHRecoveryDoesNotReplaceProvenMissingRemoteProcess(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "ps -p 4242 -o command="):
			return sshExecResult{Stderr: "no such process", ExitCode: 1}
		case command == "cat -- '/remote/session/agentctl.pid'":
			return sshExecResult{Stdout: "4242"}
		default:
			return sshExecResult{Stderr: "unexpected command", ExitCode: 127}
		}
	})
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	metadata[MetadataKeySSHAgentctlInstanceID] = "native-execution-id"
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SSH recovery is not wired to detailed candidate outcomes")
	}
	instances, outcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh", AgentExecutionID: "execution-ssh",
		Runtime: agentruntime.RuntimeSSH, PID: 4242, TransientAuthToken: "auth-token", Metadata: metadata,
	}})
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 0 || outcomes["session-ssh"] != RecoveryOutcomeNoMatchingInstance {
		t.Fatalf("proven absent SSH process = instances %d, outcome %q; want no matching instance", len(instances), outcomes["session-ssh"])
	}
	if got := server.commands(); len(got) != 2 || !strings.Contains(got[0], "ps -p 4242 -o command=") || got[1] != "cat -- '/remote/session/agentctl.pid'" {
		t.Fatalf("absent-process recovery commands = %q, want read-only process and pidfile checks", got)
	}
	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
