package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

func TestSSHWindowsTrackedStopTerminatesProcessTreeAndSessionDir(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	harness := newWindowsSSHLaunchHarness(t, 4242)
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	metadata := sshConnectionMetadata(t, harness.server)
	metadata[MetadataKeySetupScript] = "Write-Output 'win-prepared'"
	metadata[MetadataKeyCleanupScript] = "Write-Output 'KANDEV_WIN_CLEANUP_MARKER'"

	instance, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-win",
		TaskID:     "task-win",
		SessionID:  "session-win",
		Protocol:   "acp",
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	instance.StopReason = StopReasonTaskArchived
	if err := exec.StopInstance(context.Background(), instance, false); err != nil {
		t.Fatalf("StopInstance on Windows session: %v", err)
	}

	harness.mu.Lock()
	cleanupCount := len(harness.cleanupScripts)
	stopCount := len(harness.stopScripts)
	harness.mu.Unlock()
	if cleanupCount != 1 {
		t.Fatalf("Windows cleanup script runs = %d, want 1", cleanupCount)
	}
	if stopCount != 1 {
		t.Fatalf("Windows stop script runs = %d, want 1", stopCount)
	}
	if harness.alive.Load() {
		t.Fatal("expected Windows agentctl.exe process to be terminated")
	}
}

func TestSSHWindowsBackendShutdownPreservesAgentctlAndResumeReconnects(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	harness := newWindowsSSHLaunchHarness(t, 4242)
	exec1 := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec1.Close() })

	metadata := sshConnectionMetadata(t, harness.server)
	metadata[MetadataKeySetupScript] = "Write-Output 'win-prepared'"

	instance1, err := exec1.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-win",
		TaskID:     "task-win",
		SessionID:  "session-win",
		Protocol:   "acp",
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("initial CreateInstance: %v", err)
	}

	// Graceful backend shutdown must close SSH transport without killing remote agentctl.exe.
	instance1.StopReason = StopReasonBackendShutdown
	if err := exec1.StopInstance(context.Background(), instance1, false); err != nil {
		t.Fatalf("StopInstance(backend shutdown): %v", err)
	}
	if !harness.alive.Load() {
		t.Fatal("backend shutdown must preserve the remote Windows agentctl.exe process")
	}

	// Simulate backend restart: a fresh SSHExecutor resumes using persisted metadata.
	exec2 := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec2.Close() })

	resumeMeta := cloneSSHMetadata(instance1.Metadata)
	resumeReq := &ExecutorCreateRequest{
		InstanceID: "instance-win-resumed",
		TaskID:     "task-win",
		SessionID:  "session-win",
		AuthToken:  "persisted-win-token",
		Metadata:   resumeMeta,
	}
	if err := exec2.ResumeRemoteInstance(context.Background(), resumeReq); err != nil {
		t.Fatalf("ResumeRemoteInstance on Windows SSH host: %v", err)
	}

	harness.mu.Lock()
	startsBeforeCreate := len(harness.startScripts)
	harness.mu.Unlock()

	resumedInstance, err := exec2.CreateInstance(context.Background(), resumeReq)
	if err != nil {
		t.Fatalf("CreateInstance after ResumeRemoteInstance: %v", err)
	}
	harness.mu.Lock()
	startsAfterCreate := len(harness.startScripts)
	harness.mu.Unlock()
	if startsAfterCreate != startsBeforeCreate {
		t.Fatalf("resumed CreateInstance spawned %d new processes, want 0", startsAfterCreate-startsBeforeCreate)
	}
	if got := getMetadataString(resumedInstance.Metadata, MetadataKeySSHRemotePlatform); got != "windows/amd64" {
		t.Fatalf("resumed ssh_remote_platform = %q, want windows/amd64", got)
	}

	status, err := exec2.GetRemoteStatus(context.Background(), resumedInstance)
	if err != nil || status.State != sshStatusRunning {
		t.Fatalf("GetRemoteStatus while alive = (%+v, %v), want running", status, err)
	}

	// Simulate remote process exit and verify both status and ConfirmAgentctlAbsent over PowerShell.
	harness.alive.Store(false)
	statusDown, err := exec2.GetRemoteStatus(context.Background(), resumedInstance)
	if err != nil || statusDown.State != sshStatusAgentctlDown {
		t.Fatalf("GetRemoteStatus after exit = (%+v, %v), want agentctl-down", statusDown, err)
	}
	absent, err := exec2.ConfirmAgentctlAbsent(context.Background(), resumedInstance.InstanceID)
	if err != nil || !absent {
		t.Fatalf("ConfirmAgentctlAbsent = (%v, %v), want (true, nil)", absent, err)
	}
}

func TestSSHWindowsResumeClearsMetadataWhenRemoteProcessExited(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	harness := newWindowsSSHLaunchHarness(t, 4242)
	harness.alive.Store(false)

	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	wantTaskDir := `C:\Users\Kandev User\.kandev\tasks\task-task-win`
	meta := sshConnectionMetadata(t, harness.server)
	meta[MetadataKeySSHRemoteAgentctlPID] = "4242"
	meta[MetadataKeySSHRemoteAgentctlPort] = "45123"
	meta[MetadataKeySSHRemoteControlPort] = "45124"
	meta[MetadataKeySSHRemoteSessionDir] = wantTaskDir + `\.kandev\sessions\session-win`
	meta[MetadataKeySSHRemoteTaskDir] = wantTaskDir
	meta[MetadataKeySSHRemotePlatform] = "windows/amd64"

	req := &ExecutorCreateRequest{
		InstanceID: "instance-win",
		TaskID:     "task-win",
		SessionID:  "session-win",
		AuthToken:  "persisted-win-token",
		Metadata:   meta,
	}
	if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
		t.Fatalf("ResumeRemoteInstance with dead Windows process: %v", err)
	}
	if got := getMetadataString(req.Metadata, MetadataKeySSHRemoteAgentctlPID); got != "" {
		t.Fatalf("expected dead Windows PID metadata cleared, got %q", got)
	}
	if got := getMetadataString(req.Metadata, MetadataKeySSHRemoteTaskDir); got != wantTaskDir {
		t.Fatalf("expected Windows taskDir preserved across dead-PID resume, got %q", got)
	}
}

func TestSSHWindowsRecoverInstancesDetailedReattachesAndDiscoversControlPort(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	wantTaskDir := `C:\Users\Kandev User\.kandev\tasks\task-task-win`
	wantSessionDir := wantTaskDir + `\.kandev\sessions\session-win`

	controlServer := newSSHRecoveryInstanceInfoServer(t, agentctl.InstanceInfo{
		ID:            "win-native-id",
		Port:          45123,
		Status:        "running",
		WorkspacePath: `c:/users/kandev user/.kandev/tasks/task-task-win`,
		SessionID:     "session-win",
		TaskID:        "task-win",
		Env:           map[string]string{"WIN_RECOVERED": "1"},
	})

	alive := true
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		script, ok := decodePowerShellEncodedCommand(command)
		if !ok {
			return sshFail("expected PowerShell command: " + command)
		}
		switch {
		case strings.Contains(script, "Win32_Process"):
			if !alive {
				return sshOut("ABSENT\r\n")
			}
			return sshOut(`CMDLINE:"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-task-win"` + "\r\n")
		case strings.Contains(script, "DIR_MISSING"):
			if !alive {
				return sshOut("DIR_MISSING\r\n")
			}
			return sshOut("PID:4242\r\n")
		case strings.Contains(script, "agentctl.log"):
			return sshOut(`{"level":"info","msg":"HTTP server bound successfully","address":"127.0.0.1:45124"}` + "\r\n")
		default:
			return sshFail("unexpected PowerShell script: " + script)
		}
	})
	server.forwardTo(controlServer.Listener.Addr().String())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "45123"
	// Deliberately omit MetadataKeySSHRemoteControlPort to exercise Windows log discovery.
	metadata[MetadataKeySSHRemoteSessionDir] = wantSessionDir
	metadata[MetadataKeySSHRemoteTaskDir] = wantTaskDir
	metadata[MetadataKeySSHAgentctlInstanceID] = "win-native-id"

	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	records := []*models.ExecutorRunning{{
		ID:                 "session-win",
		SessionID:          "session-win",
		TaskID:             "task-win",
		AgentExecutionID:   "backend-exec-win",
		Runtime:            agentruntime.RuntimeSSH,
		PID:                4242,
		TransientAuthToken: "refreshed-auth-token",
		Metadata:           metadata,
	}}

	instances, outcomes, err := exec.RecoverInstancesDetailed(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 1 || outcomes["session-win"] != "" {
		t.Fatalf("recovered = %d, outcomes = %#v", len(instances), outcomes)
	}
	if got := getMetadataString(instances[0].Metadata, MetadataKeySSHRemoteControlPort); got != "45124" {
		t.Fatalf("discovered Windows control port = %q, want 45124", got)
	}
	if got := getMetadataString(instances[0].Metadata, MetadataKeySSHRemotePlatform); got != "windows/amd64" {
		t.Fatalf("inferred Windows platform metadata = %q, want windows/amd64", got)
	}
	instances[0].DiscardRecovery()
	instances[0].Client.Close()

	// Now verify confirmed absence outcome when the Windows process has exited.
	alive = false
	instancesAbsent, outcomesAbsent, err := exec.RecoverInstancesDetailed(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed(absent): %v", err)
	}
	if len(instancesAbsent) != 0 || outcomesAbsent["session-win"] != RecoveryOutcomeNoMatchingInstance {
		t.Fatalf("absent recovery = (%d, %#v), want NoMatchingInstance", len(instancesAbsent), outcomesAbsent)
	}
}

func TestSSHWindowsPersistedStopVerifiesIdentityAndGuardsSessionDir(t *testing.T) {
	wantTaskDir := `C:\Users\Kandev User\.kandev\tasks\task-1`
	wantSessionDir := wantTaskDir + `\.kandev\sessions\session-1`

	t.Run("confirmed Windows agentctl pid is killed and session dir removed", func(t *testing.T) {
		killed := false
		removed := false
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			script, ok := decodePowerShellEncodedCommand(command)
			if !ok {
				return sshFail("expected PowerShell: " + command)
			}
			switch {
			case strings.Contains(script, "Win32_Process"):
				return sshOut(`CMDLINE:"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-1"` + "\r\n")
			case strings.Contains(script, "DIR_MISSING"):
				return sshOut("PID:4242\r\n")
			case strings.Contains(script, "$targetPid = 4242") && strings.Contains(script, "taskkill.exe /PID $targetPid /T /F"):
				killed = true
				removed = strings.Contains(script, "Remove-Item -LiteralPath")
				return sshOK
			default:
				return sshFail("unexpected PowerShell script: " + script)
			}
		})
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteSessionDir] = wantSessionDir
		metadata[MetadataKeySSHRemoteTaskDir] = wantTaskDir

		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "persisted-win-1",
			StopReason: "stopped via API",
			Metadata:   metadata,
		}, true)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		if !killed || !removed {
			t.Fatalf("killed=%v removed=%v, want both true", killed, removed)
		}
	})

	t.Run("reused PID running unrelated Windows process skips kill and reclaims abandoned dir", func(t *testing.T) {
		killed := false
		dirRemoved := false
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			script, ok := decodePowerShellEncodedCommand(command)
			if !ok {
				return sshFail("expected PowerShell: " + command)
			}
			switch {
			case strings.Contains(script, "Win32_Process"):
				return sshOut(`CMDLINE:"C:\Windows\System32\notepad.exe"` + "\r\n")
			case strings.Contains(script, "DIR_MISSING"):
				return sshOut("PID:4242\r\n")
			case strings.Contains(script, "taskkill.exe"):
				killed = true
				return sshOK
			case strings.Contains(script, "Remove-Item -LiteralPath"):
				dirRemoved = true
				return sshOK
			default:
				return sshFail("unexpected PowerShell script: " + script)
			}
		})
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteSessionDir] = wantSessionDir
		metadata[MetadataKeySSHRemoteTaskDir] = wantTaskDir

		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "persisted-win-reused",
			StopReason: "stopped via API",
			Metadata:   metadata,
		}, true)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		if killed {
			t.Fatal("must NOT kill an unrelated Windows process holding a recycled PID")
		}
		if !dirRemoved {
			t.Fatal("expected abandoned Windows session directory to be reclaimed")
		}
	})

	t.Run("session dir claimed by newer Windows PID fails closed without killing or deleting", func(t *testing.T) {
		acted := false
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			script, ok := decodePowerShellEncodedCommand(command)
			if !ok {
				return sshFail("expected PowerShell: " + command)
			}
			switch {
			case strings.Contains(script, "Win32_Process"):
				return sshOut("ABSENT\r\n")
			case strings.Contains(script, "DIR_MISSING"):
				return sshOut("PID:9999\r\n")
			case strings.Contains(script, "taskkill.exe"), strings.Contains(script, "Remove-Item"):
				acted = true
				return sshOK
			default:
				return sshFail("unexpected PowerShell script: " + script)
			}
		})
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteSessionDir] = wantSessionDir
		metadata[MetadataKeySSHRemoteTaskDir] = wantTaskDir

		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "persisted-win-conflict",
			StopReason: "stopped via API",
			Metadata:   metadata,
		}, true)
		if err == nil {
			t.Fatal("expected StopInstance to fail closed when pidfile names a newer PID")
		}
		if acted {
			t.Fatal("must neither kill nor remove session dir owned by a newer launch")
		}
	})
}

type windowsOrphanSweepTestStore struct {
	orphanSweepRootTestStore
	sessions []*models.TaskSession
}

func (s *windowsOrphanSweepTestStore) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return s.sessions, nil
}

func TestSSHWindowsOrphanSweepInventoriesAndStopsOrphanedAgentctl(t *testing.T) {
	now := time.Now()
	store := &windowsOrphanSweepTestStore{
		orphanSweepRootTestStore: orphanSweepRootTestStore{
			task: &models.Task{ID: "archived-win", ArchivedAt: &now},
		},
		sessions: []*models.TaskSession{
			{ID: "sess-orphan", TaskID: "archived-win", State: models.TaskSessionStateCompleted},
		},
	}
	stoppedPID := false
	removedSessionDir := false
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if command == remoteHomeCommand {
			return sshFail("printf not found")
		}
		script, ok := decodePowerShellEncodedCommand(command)
		if !ok {
			return sshFail("expected PowerShell command: " + command)
		}
		switch {
		case strings.Contains(script, "$env:USERPROFILE"):
			return sshOut(`C:\Users\Kandev User`)
		case strings.Contains(script, "Get-ChildItem -LiteralPath $root -Directory -Filter 'task-*'"):
			return sshOut(
				"PROC\t7788\t100\t\"C:\\Users\\Kandev User\\.kandev\\bin\\agentctl.exe\" --workdir \"C:\\Users\\Kandev User\\.kandev\\tasks\\task-archived-win\"\r\n" +
					"PIDFILE\ttask-archived-win\tsess-orphan\t7788\r\n",
			)
		case strings.Contains(script, "Win32_Process -Filter 'ProcessId = 7788'"):
			return sshOut("CMDLINE:\"C:\\Users\\Kandev User\\.kandev\\bin\\agentctl.exe\" --workdir \"C:\\Users\\Kandev User\\.kandev\\tasks\\task-archived-win\"\r\n")
		case strings.Contains(script, "$targetPid = 7788") && strings.Contains(script, "taskkill.exe /PID $targetPid /T /F"):
			stoppedPID = true
			removedSessionDir = strings.Contains(script, `C:\Users\Kandev User\.kandev\tasks\task-archived-win\.kandev\sessions\sess-orphan`)
			return sshOK
		default:
			return sshFail("unexpected PowerShell script: " + script)
		}
	})

	report, err := sweepSSHExecutorOrphans(
		context.Background(),
		server.dial(t),
		store,
		"win-exec-1",
		map[string]string{},
		logger.Default(),
		nil,
	)
	if err != nil {
		t.Fatalf("sweepSSHExecutorOrphans on Windows: %v", err)
	}
	if report.Found != 1 || report.Stopped != 1 || !stoppedPID || !removedSessionDir {
		t.Fatalf("Windows orphan sweep report = %+v, stoppedPID=%v, removedSessionDir=%v", report, stoppedPID, removedSessionDir)
	}
}
