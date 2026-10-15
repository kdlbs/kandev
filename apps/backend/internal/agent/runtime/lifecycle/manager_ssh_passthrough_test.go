package lifecycle

import (
	"context"
	"encoding/json"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type sshPassthroughCredentialSource struct {
	token string
	done  chan struct{}
}

func (s *sshPassthroughCredentialSource) AcceptsFull(token string) bool { return token == s.token }
func (s *sshPassthroughCredentialSource) Invalidated() <-chan struct{}  { return s.done }
func (s *sshPassthroughCredentialSource) AcceptsFullWithInvalidation(token string) (bool, <-chan struct{}) {
	return s.AcceptsFull(token), s.done
}

func sshPassthroughTestController(t *testing.T, execution *AgentExecution) (*process.Manager, string) {
	return sshPassthroughTestControllerWithHandler(t, execution, nil)
}

func sshPassthroughTestControllerWithHandler(t *testing.T, execution *AgentExecution, wrap func(http.Handler) http.Handler) (*process.Manager, string) {
	t.Helper()
	log := newTestRegistryLogger()
	root := t.TempDir()
	cfg := &config.InstanceConfig{WorkDir: root, SessionID: execution.SessionID, InstanceID: execution.ID, Port: 43123, AuthToken: "controller-token"}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
	source := &sshPassthroughCredentialSource{token: cfg.AuthToken, done: make(chan struct{})}
	srv := api.NewServer(cfg, manager, nil, nil, log)
	srv.SetCredentialSource(source)
	handler := srv.Router()
	if wrap != nil {
		handler = wrap(handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	client := agentctl.NewClient(parsed.Hostname(), port, log, agentctl.WithExecutionID(execution.ID), agentctl.WithSessionID(execution.SessionID), agentctl.WithAuthToken(cfg.AuthToken))
	t.Cleanup(client.Close)
	t.Cleanup(func() { close(source.done) })
	execution.agentctl = client
	execution.ExecutorType = "ssh"
	execution.RuntimeName = agentruntime.RuntimeSSH
	execution.standalonePort = 0
	execution.deleteMetadataValues("standalone_port")
	execution.setMetadataValue(MetadataKeySSHRemoteAgentctlPort, strconv.Itoa(cfg.Port))
	return manager, root
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.5
func TestSSHPassthroughBootstrapUsesRemoteMCP(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	_, remoteRoot := sshPassthroughTestController(t, execution)
	_, _, _, cmd, err := mgr.passthroughAgentCommandWithPreparation(context.Background(), execution, profile, nil)
	require.NoError(t, err)
	var configPath string
	for index, arg := range cmd.Args() {
		if arg == "--mcp-config" {
			configPath = cmd.Args()[index+1]
			break
		}
	}
	require.True(t, strings.HasPrefix(configPath, remoteRoot+string(filepath.Separator)), configPath)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "http://127.0.0.1:43123/mcp")
	require.NoDirExists(t, filepath.Join(mgr.dataDir, "passthrough-mcp"))
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.1
func TestSSHPassthroughNeverStartsHostCLI(t *testing.T) {
	for _, matchingHostPath := range []bool{false, true} {
		t.Run(strconv.FormatBool(matchingHostPath), func(t *testing.T) {
			mgr, hostRunner, execution := newPassthroughRunnerManager(t)
			controller, remoteRoot := sshPassthroughTestController(t, execution)
			execution.WorkspacePath = filepath.Join(t.TempDir(), "host-workspace")
			if matchingHostPath {
				require.NoError(t, os.MkdirAll(execution.WorkspacePath, 0o700))
			}
			info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "printf remote > remote-marker; cat"), nil)
			if info != nil {
				t.Cleanup(func() {
					if _, exists := hostRunner.Get(info.ID, false); exists {
						_ = hostRunner.Stop(context.Background(), info.ID)
					}
				})
			}
			require.NoError(t, err)
			require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(remoteRoot, "remote-marker")); return err == nil }, 5*time.Second, 10*time.Millisecond)
			require.NoFileExists(t, filepath.Join(execution.WorkspacePath, "remote-marker"))
			require.NotZero(t, controller.Passthrough().Snapshot().OSPID)
		})
	}
}

func TestSSHPassthroughRecoveryUsesPTYEvidence(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	execution.setMetadataValue(MetadataKeySSHAgentctlInstanceID, execution.ID)
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "cat"), nil)
	require.NoError(t, err)
	instance := &ExecutorInstance{InstanceID: execution.ID, TaskID: execution.TaskID, SessionID: execution.SessionID, RuntimeName: agentruntime.RuntimeSSH, Client: execution.agentctl, Metadata: execution.MetadataSnapshot()}
	entry := &remoteRecoveryRetry{record: &models.ExecutorRunning{TaskID: execution.TaskID, SessionID: execution.SessionID, AgentExecutionID: execution.ID, Runtime: agentruntime.RuntimeSSH}, snapshot: &RemoteRecoverySnapshot{IsPassthrough: true}}
	require.NoError(t, mgr.proveRecoveredRemoteInstance(context.Background(), entry, instance))
	require.Equal(t, info.ID, controller.Passthrough().Snapshot().ProcessID)
}

func TestSSHPassthroughEnvironmentUsesSSHPolicy(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	execution.ExecutorType = executorTypeSSH
	execution.RuntimeName = agentruntime.RuntimeSSH
	execution.setRuntimeEnvironment(map[string]string{"HOST_SECRET": "must-stay-local", "PATH": "/host/bin", "OPENAI_API_KEY": "selected-profile-key"})
	env, err := mgr.buildPassthroughEnv(context.Background(), execution, nil)
	require.NoError(t, err)
	require.NotContains(t, env, "HOST_SECRET")
	require.NotContains(t, env, "PATH")
	require.Equal(t, "selected-profile-key", env["OPENAI_API_KEY"])
	require.Equal(t, execution.SessionID, env["KANDEV_SESSION_ID"])
}

func TestSSHPassthroughUnknownControllerDoesNotClearOwnership(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	sshPassthroughTestController(t, execution)
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	execution.agentctl = agentctl.NewClient("127.0.0.1", 1, newTestRegistryLogger())
	t.Cleanup(execution.agentctl.Close)
	require.True(t, mgr.passthroughRunnerForExecution(execution).IsProcessReadyOrPending(info.ID), "an unavailable controller cannot prove that its CLI is absent")
}

func TestSSHPassthroughRecoversLostStartResponse(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestControllerWithHandler(t, execution, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/passthrough/start" {
				next.ServeHTTP(httptest.NewRecorder(), r)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "cat"), nil)
	require.NoError(t, err, "inspect the start operation before admitting another CLI")
	require.Equal(t, controller.Passthrough().Snapshot().ProcessID, info.ID)
}

func TestSSHPassthroughRecoversLostInputResponse(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	_, root := sshPassthroughTestControllerWithHandler(t, execution, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/passthrough/input" {
				next.ServeHTTP(httptest.NewRecorder(), r)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "while IFS= read -r line; do printf '%s\\n' \"$line\" >> accepted; done"), nil)
	require.NoError(t, err)
	runner := mgr.passthroughRunnerForExecution(execution)
	require.NoError(t, runner.WriteStdin(info.ID, "once\r"), "inspect the remote receipt without writing the input again")
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(filepath.Join(root, "accepted"))
		return err == nil && string(data) == "once\n"
	}, time.Second, time.Millisecond)
}

func TestSSHPassthroughUncertainInitialPromptKeepsCompletionBarrier(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	sshPassthroughTestControllerWithHandler(t, execution, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/passthrough/input" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	execution.setMetadataValue("task_description", "initial-one")
	pt := agents.PassthroughConfig{IdleTimeout: 20 * time.Millisecond}
	info, err := mgr.startInteractiveProcess(context.Background(), execution, pt, nil, agents.NewCommand("/bin/sh", "-c", "printf ready; cat"), nil)
	require.NoError(t, err)
	data, err := json.Marshal(remotePassthroughRecordFor(execution))
	require.NoError(t, err)
	var recordBefore map[string]any
	require.NoError(t, json.Unmarshal(data, &recordBefore))
	require.Equal(t, "pending", recordBefore["initial_prompt_state"], "save the prompt boundary before launch acknowledgement")
	require.Equal(t, float64(1), recordBefore["initial_expected_chunks"])
	execution.PassthroughProcessID = info.ID
	execution.passthroughInitialPromptProcessID = info.ID
	mgr.autoInjectInitialPrompt(execution, pt, info.ID)
	require.Equal(t, "uncertain", remotePassthroughRecordFor(execution).InitialPromptState)
	require.Equal(t, info.ID, execution.passthroughInitialPromptProcessID, "idle cannot complete an uncertain initial submission")
	require.Len(t, remotePassthroughRecordFor(execution).InitialSubmissionIDs, 1)
}

func TestSSHPassthroughUnsupportedHelperHasActionableFailure(t *testing.T) {
	execution := &AgentExecution{SessionID: "session-1"}
	failure := bootstrapFailureFor(execution, agentctl.ErrPassthroughUnsupported)
	require.Equal(t, models.AgentErrorCauseCodeTransportUnavailable, failure.SafeCode())
	require.Contains(t, failure.SafeDetail(), "Update Kandev")
}

func TestSSHPassthroughBackendLogsExcludePromptAndCLIValues(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "mock-agent")
	require.NoError(t, mgr.executionStore.Add(execution))
	sshPassthroughTestController(t, execution)
	execution.setMetadataValue("task_description", "private-prompt-value")
	profile.CLIFlags = []settingsmodels.CLIFlag{{Flag: "--profile-token=private-cli-value", Enabled: true}}
	core, observed := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	mgr.logger = log
	_ = mgr.startPassthroughSession(context.Background(), execution, profile)
	t.Cleanup(func() { _ = mgr.Stop() })
	require.NotEmpty(t, observed.FilterMessage("passthrough command built").All(), "the log boundary must be exercised")
	for _, entry := range observed.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private-prompt-value")
		require.NotContains(t, string(encoded), "private-cli-value")
	}
}

func TestSSHPassthroughFailedResetPreservesPriorProcessState(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestControllerWithHandler(t, execution, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	execution.PassthroughStartedAt = info.StartedAt
	execution.passthroughInitialPromptProcessID = info.ID
	require.Error(t, mgr.restartPassthroughProcess(context.Background(), execution))
	require.Equal(t, info.ID, execution.PassthroughProcessID)
	require.Equal(t, info.StartedAt, execution.PassthroughStartedAt)
	require.Equal(t, info.ID, execution.passthroughInitialPromptProcessID)
	require.Equal(t, info.ID, controller.Passthrough().Snapshot().ProcessID)
	require.Equal(t, uint64(1), controller.Passthrough().Snapshot().Generation)
}

func TestSSHPassthroughPersistsAcceptedEventCursor(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/bin/sh", "-c", "cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	require.NoError(t, mgr.passthroughRunnerForExecution(execution).WriteStdin(info.ID, "cursor-private-data\r"))
	page, err := controller.Passthrough().Events(context.Background(), 0, false)
	require.NoError(t, err)
	var submitted agentctltypes.PassthroughEvent
	for _, event := range page.Events {
		if event.Kind == "submitted" {
			submitted = event
		}
	}
	require.NotZero(t, submitted.Sequence)
	mgr.handleRemotePassthroughEvent(execution, submitted)
	record := remotePassthroughRecordFor(execution)
	require.Equal(t, submitted.Sequence, record.EventSequence)
	persisted := FilterPersistentMetadata(execution.MetadataSnapshot())
	require.Contains(t, persisted, metadataKeySSHPassthroughState)
	encoded, err := json.Marshal(persisted[metadataKeySSHPassthroughState])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "cursor-private-data")
	require.NotContains(t, string(encoded), "controller-token")
}

func TestSSHPassthroughResumeReconcilesWrittenInitialPrompt(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	var failedReceipt atomic.Bool
	_, root := sshPassthroughTestControllerWithHandler(t, execution, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/passthrough/input" {
				next.ServeHTTP(httptest.NewRecorder(), r)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if strings.Contains(r.URL.Path, "/submissions/") && !failedReceipt.Swap(true) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	t.Cleanup(func() { _ = mgr.Stop() })
	execution.setMetadataValue("task_description", "initial-once")
	pt := agents.PassthroughConfig{IdleTimeout: 20 * time.Millisecond}
	info, err := mgr.startInteractiveProcess(context.Background(), execution, pt, nil, agents.NewCommand("/bin/sh", "-c", "printf ready; while IFS= read -r line; do printf '%s\n' \"$line\" >> accepted; done"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	execution.passthroughInitialPromptProcessID = info.ID
	mgr.autoInjectInitialPrompt(execution, pt, info.ID)
	require.Equal(t, "uncertain", remotePassthroughRecordFor(execution).InitialPromptState)
	require.NoError(t, mgr.ResumePassthroughSession(context.Background(), execution.SessionID))
	require.Equal(t, "written", remotePassthroughRecordFor(execution).InitialPromptState)
	require.Empty(t, execution.passthroughInitialPromptProcessID)
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(filepath.Join(root, "accepted"))
		return err == nil && string(data) == "initial-once\n"
	}, time.Second, time.Millisecond)
}

func TestSSHMockPassthroughProvidesRemoteMCPConfig(t *testing.T) {
	mgr, execution, profile := newPassthroughMCPTestManager(t, "mock-agent")
	_, root := sshPassthroughTestController(t, execution)
	_, _, _, command, err := mgr.passthroughAgentCommandWithPreparation(context.Background(), execution, profile, nil)
	require.NoError(t, err)
	var configPath string
	for index, arg := range command.Args() {
		if arg == "--mcp-config" {
			configPath = command.Args()[index+1]
		}
	}
	require.NotEmpty(t, configPath, "the TUI fixture must use the real remote MCP endpoint")
	require.True(t, strings.HasPrefix(configPath, root+string(filepath.Separator)))
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "http://127.0.0.1:43123/mcp")
}

func TestSSHPassthroughMissingCLIHasActionableFailure(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	sshPassthroughTestController(t, execution)
	_, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("/missing-cli-private-value"), nil)
	require.Error(t, err)
	failure := bootstrapFailureFor(execution, err)
	require.Contains(t, failure.SafeDetail(), "installed and executable")
	require.NotContains(t, failure.SafeDetail(), "private-value")
}

func TestSSHPassthroughReplayGapRequiresAuthenticatedSnapshot(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{IdleTimeout: 20 * time.Millisecond}, nil, agents.NewCommand("/bin/sh", "-c", "printf ready; cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	snapshot := controller.Passthrough().Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, controller.Passthrough().FirstIdle(ctx, snapshot.PassthroughTarget))
	snapshot = controller.Passthrough().Snapshot()
	require.True(t, snapshot.WaitingForInput)
	require.NoError(t, mgr.MarkPassthroughRunning(execution.SessionID))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, upgradeErr := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if upgradeErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteJSON(agentctltypes.PassthroughEventPage{Gap: true, Sequence: 999})
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	after := mgr.consumeRemotePassthroughEvents(conn, execution, execution.agentctl, snapshot, 0)
	require.Equal(t, snapshot.Sequence, after, "only an authenticated snapshot can establish the new cursor")
	require.Equal(t, snapshot.Sequence, remotePassthroughRecordFor(execution).EventSequence)
	require.Equal(t, info.ID, execution.PassthroughProcessID)
	require.Equal(t, "RUNNING", string(execution.Status), "a gap cannot synthesize a lost turn completion")
}

func TestSSHPassthroughTerminalJoinsInitialStartup(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	sshPassthroughTestController(t, execution)
	t.Cleanup(func() { _ = mgr.Stop() })
	execution.IsPassthrough = true
	entered, release := make(chan struct{}), make(chan struct{})
	startupDone := make(chan error, 1)
	go func() {
		_, err := mgr.doCoalescedExecution(context.Background(), "agent-start:"+execution.ID, func(context.Context) (interface{}, error) {
			close(entered)
			<-release
			return nil, nil
		})
		startupDone <- err
	}()
	<-entered
	defer func() { close(release); require.NoError(t, <-startupDone) }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := mgr.EnsurePassthroughExecution(ctx, execution.SessionID)
	require.ErrorIs(t, err, context.DeadlineExceeded, "terminal attachment must wait for the existing startup instead of launching a resume CLI")
	require.Empty(t, remotePassthroughRecordFor(execution).StartOperationID, "the terminal must not submit another start operation")
}

func TestSSHPassthroughFirstLaunchAlreadyStartedByTerminal(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	memoryBus := eventbus.NewMemoryEventBus(newTestRegistryLogger())
	t.Cleanup(memoryBus.Close)
	mgr.eventBus = memoryBus
	mgr.eventPublisher = NewEventPublisher(memoryBus, newTestRegistryLogger())
	controller, _ := sshPassthroughTestController(t, execution)
	t.Cleanup(func() { _ = mgr.Stop() })
	const agentID = "ssh-initial-launch-test"
	require.NoError(t, mgr.registry.Register(&testAgent{
		id: agentID, enabled: true,
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported: true, PassthroughCmd: agents.NewCommand("sh", "-c", "cat"),
		}},
	}))
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: agentID, cliPassthrough: true}
	execution.AgentID = agentID
	require.NoError(t, mgr.startPassthroughSession(context.Background(), execution, nil))
	first := controller.Passthrough().Snapshot()
	require.NoError(t, mgr.startPassthroughSession(context.Background(), execution, nil), "a later first-launch caller must adopt the completed terminal launch")
	require.Equal(t, first.PassthroughTarget, controller.Passthrough().Snapshot().PassthroughTarget)
	require.Equal(t, first.ProcessID, execution.PassthroughProcessID)
}

func TestSSHPassthroughFallbackPersistsInitialPromptBeforeStart(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	t.Cleanup(func() { _ = mgr.Stop() })
	const agentID = "ssh-fallback-prompt-test"
	require.NoError(t, mgr.registry.Register(&testAgent{
		id: agentID, enabled: true,
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported: true, PassthroughCmd: agents.NewCommand("sh", "-c", "cat"),
			AutoInjectPrompt: true, SubmitSequence: "\r", IdleTimeout: time.Second,
		}},
	}))
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: agentID, cliPassthrough: true}
	execution.AgentID = agentID
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("sh", "-c", "cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	_, _, detach, err := controller.Passthrough().SubscribeTerminal(controller.Passthrough().Snapshot().PassthroughTarget)
	require.NoError(t, err)
	defer detach()
	require.NoError(t, controller.Passthrough().Stop(context.Background(), controller.Passthrough().Snapshot().PassthroughTarget))
	execution.setMetadataValue("task_description", "initial after fallback")
	mgr.attemptResumeFallback(execution, mgr.passthroughRunnerForExecution(execution), execution.SessionID, 1, time.Second)
	record := remotePassthroughRecordFor(execution)
	require.Equal(t, 1, record.InitialExpectedChunks)
	require.Equal(t, "pending", record.InitialPromptState)
}

func TestSSHPassthroughRecoveryRejectsMissingStartEvidence(t *testing.T) {
	snapshot := agentctltypes.PassthroughSnapshot{
		PassthroughTarget: agentctltypes.PassthroughTarget{ProcessID: "remote-process", Generation: 3},
		InstanceID:        "controller", OperationID: "start-operation",
	}
	for _, record := range []remotePassthroughRecord{
		{},
		{InstanceID: "controller", StartOperationID: "another-operation", PassthroughTarget: snapshot.PassthroughTarget},
		{InstanceID: "controller", StartOperationID: "start-operation", PassthroughTarget: agentctltypes.PassthroughTarget{Generation: 2}},
	} {
		_, err := reconcileRemoteInitialPrompt(context.Background(), nil, snapshot, record)
		require.ErrorIs(t, err, agentctl.ErrPassthroughConflict)
	}
}

func TestSSHPassthroughStaleCleanupDoesNotReenterClientLease(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	mgr.executorRegistry.Register(&MockExecutor{name: executor.NameSSH})
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("sh", "-c", "cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	require.NoError(t, mgr.cleanupStaleExecution(context.Background(), execution))
	require.False(t, controller.Passthrough().Live())
	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists)
}

func TestSSHPassthroughDelayedStartupIdleCannotCompleteInitialTurn(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	execution.setMetadataValue("task_description", "initial work")
	pt := agents.PassthroughConfig{IdleTimeout: 20 * time.Millisecond}
	info, err := mgr.startInteractiveProcess(context.Background(), execution, pt, nil, agents.NewCommand("sh", "-c", "printf ready; cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	execution.passthroughInitialPromptProcessID = info.ID
	mgr.autoInjectInitialPrompt(execution, pt, info.ID)
	require.Empty(t, execution.passthroughInitialPromptProcessID)
	page, err := controller.Passthrough().Events(context.Background(), 0, false)
	require.NoError(t, err)
	var startup agentctltypes.PassthroughEvent
	var submitted agentctltypes.PassthroughEvent
	for _, event := range page.Events {
		if event.Kind == "idle" && startup.Sequence == 0 {
			startup = event
		}
		if event.Kind == "submitted" {
			submitted = event
		}
	}
	require.NotZero(t, startup.Sequence)
	require.Greater(t, submitted.Sequence, startup.Sequence)
	mgr.handleRemotePassthroughEvent(execution, startup)
	require.Equal(t, "RUNNING", string(execution.Status), "delayed startup idle must not release the active turn")
	completed := startup
	completed.Sequence = submitted.Sequence + 1
	mgr.handleRemotePassthroughEvent(execution, completed)
	require.Equal(t, "READY", string(execution.Status), "a later idle must still complete the turn")
}

func TestSSHPassthroughObserverReleasesTargetWhenClientIsAbsent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr, _, execution := newPassthroughRunnerManager(t)
		execution.ExecutorType = "ssh"
		execution.remotePassthrough.snapshot.PassthroughTarget = agentctltypes.PassthroughTarget{ProcessID: "process", Generation: 1}
		mgr.observeRemotePassthrough(execution)
		synctest.Wait()
		mgr.wg.Wait()
		execution.remotePassthroughMu.Lock()
		defer execution.remotePassthroughMu.Unlock()
		require.Empty(t, execution.remotePassthrough.observerTarget.ProcessID, "an exited observer must not prevent reconnection")
		require.Nil(t, execution.remotePassthrough.observerCancel)
	})
}

func TestSSHPassthroughReplayGapProjectsVerifiedExit(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	controller, _ := sshPassthroughTestController(t, execution)
	info, err := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, agents.NewCommand("sh", "-c", "cat"), nil)
	require.NoError(t, err)
	execution.PassthroughProcessID = info.ID
	source := controller.Passthrough().Snapshot()
	require.NoError(t, controller.Passthrough().Stop(context.Background(), source.PassthroughTarget))
	require.Eventually(t, func() bool { return controller.Passthrough().Snapshot().Status != agentctltypes.ProcessStatusRunning }, time.Second, time.Millisecond)
	bus := &MockEventBus{}
	mgr.eventPublisher = NewEventPublisher(bus, newTestRegistryLogger())
	mgr.shuttingDown.Store(true)
	after := mgr.reconcileRemotePassthroughGap(execution, execution.agentctl, source, source.StartSequence)
	require.Greater(t, after, source.StartSequence)
	require.Len(t, bus.PublishedEvents, 1, "a verified exit must reach normal status handling")
	require.Equal(t, events.ProcessStatus, bus.PublishedEvents[0].Type)
}

func TestSSHPassthroughStaleObserverCannotClearReplacement(t *testing.T) {
	_, _, execution := newPassthroughRunnerManager(t)
	target := agentctltypes.PassthroughTarget{ProcessID: "same-process", Generation: 1}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	execution.remotePassthrough.observerGeneration = 2
	execution.remotePassthrough.observerTarget = target
	execution.remotePassthrough.observerCancel = cancel
	execution.releaseRemotePassthroughObserver(1)
	require.Equal(t, target, execution.remotePassthrough.observerTarget)
	require.NotNil(t, execution.remotePassthrough.observerCancel)
	execution.releaseRemotePassthroughObserver(2)
	require.Empty(t, execution.remotePassthrough.observerTarget.ProcessID)
	require.Nil(t, execution.remotePassthrough.observerCancel)
}
