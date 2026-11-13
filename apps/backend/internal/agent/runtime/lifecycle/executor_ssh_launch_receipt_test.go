package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func testSSHLaunchOwner() sshlaunch.Owner {
	return sshlaunch.Owner{
		TaskID:               "task-1",
		SessionID:            "session-1",
		ControllerInstanceID: "instance-1",
		TaskDir:              "/home/kandev/.kandev/tasks/task-1",
		SessionDir:           "/home/kandev/.kandev/tasks/task-1/sessions/session-1",
		Target: sshlaunch.TargetBinding{
			Host:            "runner.example.test",
			Port:            "22",
			User:            "kandev",
			HostFingerprint: "SHA256:test",
		},
	}
}

func newTestSSHLaunchReceipt(owner sshlaunch.Owner, pid int) sshlaunch.Receipt {
	generation, err := sshlaunch.NewGeneration()
	if err != nil {
		panic(err)
	}
	return sshlaunch.Receipt{
		Version:    sshlaunch.Version,
		Generation: generation,
		Owner:      owner,
		UID:        1000,
		Process: processidentity.Identity{
			PID:        pid,
			GroupID:    pid,
			SessionID:  pid,
			BirthToken: "linux:11111111-2222-3333-4444-555555555555:987654",
		},
	}
}

func testSSHLaunchReceipt(t *testing.T, owner sshlaunch.Owner, pid int) sshlaunch.Receipt {
	t.Helper()
	receipt := newTestSSHLaunchReceipt(owner, pid)
	require.NoError(t, receipt.Validate())
	return receipt
}

func testSSHLaunchMetadata(receipt sshlaunch.Receipt) map[string]interface{} {
	metadata := map[string]interface{}{
		MetadataKeySSHHost:               receipt.Owner.Target.Host,
		MetadataKeySSHPort:               receipt.Owner.Target.Port,
		MetadataKeySSHUser:               receipt.Owner.Target.User,
		MetadataKeySSHHostFingerprint:    receipt.Owner.Target.HostFingerprint,
		MetadataKeySSHRemoteTaskDir:      receipt.Owner.TaskDir,
		MetadataKeySSHRemoteSessionDir:   receipt.Owner.SessionDir,
		MetadataKeySSHRemoteControlPort:  "43122",
		MetadataKeySSHRemoteAgentctlPort: "43123",
		MetadataKeySSHRemoteAgentctlPID:  "4242",
		MetadataKeySSHAgentctlInstanceID: receipt.Owner.ControllerInstanceID,
		MetadataKeySSHWorkdirRoot:        "/home/kandev/.kandev",
		MetadataKeyIsRemote:              true,
	}
	applySSHLaunchReceiptMetadata(metadata, &receipt)
	return metadata
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
func TestSSHLaunchReceiptSurvivesPersistenceAndResume(t *testing.T) {
	owner := testSSHLaunchOwner()
	receipt := testSSHLaunchReceipt(t, owner, 4242)

	// Persistence boundary: the same metadata a launched execution carries is
	// filtered, written as JSON, and read back as a runtime row.
	execution := &AgentExecution{
		ID:          "execution-1",
		TaskID:      owner.TaskID,
		SessionID:   owner.SessionID,
		RuntimeName: agentruntime.RuntimeSSH,
		Status:      v1.AgentStatusRunning,
	}
	for key, value := range testSSHLaunchMetadata(receipt) {
		execution.setMetadataValue(key, value)
	}
	running := buildRunningFromExecution(execution, nil)
	raw, err := json.Marshal(running)
	require.NoError(t, err)
	var decoded models.ExecutorRunning
	require.NoError(t, json.Unmarshal(raw, &decoded))

	persisted, ok := sshlaunch.ReceiptFromMetadata(decoded.Metadata)
	require.True(t, ok, "persisted row must carry the launch receipt")
	require.Equal(t, receipt.Generation, persisted.Generation)
	require.Equal(t, receipt.Process, persisted.Process)
	require.Equal(t, receipt.UID, persisted.UID)

	// Same-controller resume: the local execution ID changes, the controller
	// identity does not.
	executor := &SSHExecutor{logger: newNopLogger(t)}
	instance := executor.buildResumedInstance(
		&ExecutorCreateRequest{
			InstanceID: "execution-2",
			TaskID:     owner.TaskID,
			SessionID:  owner.SessionID,
			Metadata:   decoded.Metadata,
		},
		&sshSessionState{
			target:        &SSHTarget{Host: owner.Target.Host, Port: 22, User: owner.Target.User, PinnedFingerprint: owner.Target.HostFingerprint},
			forwarder:     &SSHPortForwarder{localPort: 43124},
			pid:           4242,
			remoteDir:     owner.SessionDir,
			remoteTaskDir: owner.TaskDir,
			authToken:     "token",
			launchReceipt: &persisted,
			controlPort:   43122,
		},
	)
	require.Equal(t, "execution-2", instance.InstanceID)
	require.Equal(t, owner.ControllerInstanceID, instance.Metadata[MetadataKeySSHAgentctlInstanceID])
	resumed, ok := sshlaunch.ReceiptFromMetadata(instance.Metadata)
	require.True(t, ok, "resume must preserve the receipt")
	require.Equal(t, receipt.Generation, resumed.Generation)
	require.Equal(t, receipt.Process, resumed.Process)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
func TestSSHLaunchReceiptReplacementDoesNotInheritPriorProof(t *testing.T) {
	owner := testSSHLaunchOwner()
	receipt := testSSHLaunchReceipt(t, owner, 4242)
	metadata := testSSHLaunchMetadata(receipt)

	// Replacement reset clears the receipt with the rest of the session-scoped
	// runtime metadata.
	clearSSHResumeRuntimeMetadata(metadata)
	require.NotContains(t, metadata, MetadataKeySSHLaunchReceipt)
	_, ok := sshlaunch.ReceiptFromMetadata(metadata)
	require.False(t, ok)

	// A prior row's receipt must not be resurrected by the persistence merge.
	prior := &models.ExecutorRunning{
		AgentExecutionID: "execution-1",
		TaskID:           owner.TaskID,
		SessionID:        owner.SessionID,
		Metadata:         testSSHLaunchMetadata(receipt),
	}
	current := &AgentExecution{
		ID:          "execution-2",
		TaskID:      owner.TaskID,
		SessionID:   owner.SessionID,
		RuntimeName: agentruntime.RuntimeSSH,
		Status:      v1.AgentStatusRunning,
	}
	current.setMetadataValue(MetadataKeySSHRemoteTaskDir, owner.TaskDir)
	merged := buildRunningFromExecution(current, prior)
	require.NotContains(t, merged.Metadata, MetadataKeySSHLaunchReceipt,
		"a replacement execution that cleared its receipt must not inherit the predecessor's proof")

	// Conflicting receipts are an error, never silent legacy evidence.
	conflicting := testSSHLaunchReceipt(t, owner, 4242)
	conflicting.Owner.ControllerInstanceID = "instance-2"
	conflictingMetadata, err := conflicting.Metadata()
	require.NoError(t, err)
	row := testSSHLaunchMetadata(receipt)
	row[MetadataKeySSHLaunchReceipt] = conflictingMetadata
	_, err = persistedSSHLaunchReceipt(row, owner.TaskID, owner.SessionID)
	require.ErrorIs(t, err, sshlaunch.ErrReceiptMismatch)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHLaunchReceiptProducerBindsGenerationAndSelf(t *testing.T) {
	owner := testSSHLaunchOwner()
	var binding sshlaunch.Binding
	handshake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receipt := newTestSSHLaunchReceipt(binding.Owner, 4242)
		receipt.Generation = binding.Generation
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "issued-token", "launch": receipt})
	}))
	defer handshake.Close()

	server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		switch {
		case strings.Contains(command, "nohup"):
			for _, line := range strings.Split(stdin, "\n") {
				raw, ok := strings.CutPrefix(line, commonconfig.InternalAgentctlStartupConfigEnv+"=")
				if !ok {
					continue
				}
				decoded, err := commonconfig.DecodeAgentctlStartupConfig(strings.Trim(strings.TrimSpace(raw), "'"))
				if err != nil {
					t.Errorf("decode startup config: %v", err)
					return sshFail("bad startup config")
				}
				if decoded.SSHLaunch == nil {
					t.Error("launch env carried no SSH launch binding")
					return sshFail("missing binding")
				}
				binding = *decoded.SSHLaunch
			}
			return sshOut("4242\n")
		case strings.Contains(command, "agentctl.log"):
			return sshOut("HTTP server bound successfully")
		default:
			return sshOK
		}
	})
	server.forwardTo(strings.TrimPrefix(handshake.URL, "http://"))

	executor := &SSHExecutor{logger: newNopLogger(t), sessions: map[string]*sshSessionState{}}
	startup := commonconfig.AgentctlStartupConfig{
		Configured:                true,
		IdleReaperInterval:        time.Minute,
		NotificationQueueCapacity: 4096,
	}
	port, pid, token, receipt, err := executor.startAgentctlAndHandshake(
		context.Background(), server.dial(t), "bash", "/home/kandev/.kandev/bin/agentctl",
		owner.TaskDir, owner.SessionDir, owner, true, &ExecutorCreateRequest{
			InstanceID:            owner.ControllerInstanceID,
			TaskID:                owner.TaskID,
			SessionID:             owner.SessionID,
			AgentctlStartupConfig: startup,
		},
	)
	require.NoError(t, err)
	require.Positive(t, port)
	require.Equal(t, 4242, pid)
	require.Equal(t, "issued-token", token)
	require.NotNil(t, receipt)
	require.Equal(t, binding.Generation, receipt.Generation)
	require.Equal(t, owner, receipt.Owner)
	require.NoError(t, receipt.ValidateBinding(binding))

	// A receipt for any other PID is a launch error, not a weaker proof.
	otherPID := *receipt
	otherPID.Process.PID = 4343
	_, err = validateLaunchedReceipt(&otherPID, binding, pid)
	require.Error(t, err)
	_, err = validateLaunchedReceipt(nil, binding, pid)
	require.Error(t, err)
	otherGeneration := binding
	otherGeneration.Generation, err = sshlaunch.NewGeneration()
	require.NoError(t, err)
	_, err = validateLaunchedReceipt(receipt, otherGeneration, pid)
	require.ErrorIs(t, err, sshlaunch.ErrReceiptMismatch)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHRecoveryRequiresTheRecordsOwnLaunchReceipt(t *testing.T) {
	owner := testSSHLaunchOwner()
	receipt := testSSHLaunchReceipt(t, owner, 4242)
	record := func(receipt sshlaunch.Receipt) *models.ExecutorRunning {
		metadata := testSSHLaunchMetadata(receipt)
		metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
		return &models.ExecutorRunning{
			SessionID:          owner.SessionID,
			TaskID:             owner.TaskID,
			AgentExecutionID:   "execution-1",
			PID:                4242,
			TransientAuthToken: "token",
			Metadata:           metadata,
		}
	}

	identity, ok := sshRecoveryIdentity(record(receipt))
	require.True(t, ok)
	require.NotNil(t, identity.launchReceipt)
	require.Equal(t, receipt.Generation, identity.launchReceipt.Generation)

	foreign := receipt
	foreign.Owner.SessionID = "session-2"
	if _, ok := sshRecoveryIdentity(record(foreign)); ok {
		t.Fatal("recovery accepted a receipt for another session")
	}

	mismatchedPID := record(receipt)
	mismatchedPID.PID = 4343
	if _, ok := sshRecoveryIdentity(mismatchedPID); ok {
		t.Fatal("recovery accepted a record whose receipt names a different pid")
	}

	// A legacy record without proof still recovers through the guarded path.
	legacy := record(receipt)
	delete(legacy.Metadata, MetadataKeySSHLaunchReceipt)
	identity, ok = sshRecoveryIdentity(legacy)
	require.True(t, ok)
	require.Nil(t, identity.launchReceipt)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
func TestSSHLaunchReceiptAbsentForNonLinuxPlatforms(t *testing.T) {
	handshake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "issued-token"})
	}))
	defer handshake.Close()

	server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		switch {
		case strings.Contains(command, "nohup"):
			for _, line := range strings.Split(stdin, "\n") {
				raw, ok := strings.CutPrefix(line, commonconfig.InternalAgentctlStartupConfigEnv+"=")
				if !ok {
					continue
				}
				decoded, err := commonconfig.DecodeAgentctlStartupConfig(strings.Trim(strings.TrimSpace(raw), "'"))
				if err != nil {
					t.Errorf("decode startup config: %v", err)
					return sshFail("bad startup config")
				}
				if decoded.SSHLaunch != nil {
					t.Error("a non-Linux remote launch must not carry an SSH launch binding")
					return sshFail("unexpected binding")
				}
			}
			return sshOut("4242\n")
		case strings.Contains(command, "agentctl.log"):
			return sshOut("HTTP server bound successfully")
		default:
			return sshOK
		}
	})
	server.forwardTo(strings.TrimPrefix(handshake.URL, "http://"))

	owner := testSSHLaunchOwner()
	executor := &SSHExecutor{logger: newNopLogger(t), sessions: map[string]*sshSessionState{}}
	_, _, token, receipt, err := executor.startAgentctlAndHandshake(
		context.Background(), server.dial(t), "zsh", "/home/kandev/.kandev/bin/agentctl",
		owner.TaskDir, owner.SessionDir, owner, false, &ExecutorCreateRequest{
			InstanceID: owner.ControllerInstanceID, TaskID: owner.TaskID, SessionID: owner.SessionID,
			AgentctlStartupConfig: commonconfig.AgentctlStartupConfig{
				Configured: true, IdleReaperInterval: time.Minute, NotificationQueueCapacity: 4096,
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, "issued-token", token)
	require.Nil(t, receipt)
}
