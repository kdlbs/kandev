package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/lsp/protocol"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

type fakeProcessManager struct {
	mu              sync.Mutex
	started         int
	stopped         int
	active          map[string]*fakeLSPProcess
	servers         []*fakeLSPServer
	serverFactory   func(int) *fakeLSPServer
	overlapObserved bool
	ownedReleased   chan struct{}
	releaseOnce     sync.Once
}

type fakeLSPProcess struct {
	id           string
	serverInput  *io.PipeReader
	serverOutput *io.PipeWriter
	done         chan struct{}
	stopOnce     sync.Once
}

type fakeLSPServer struct {
	holdInitialize       chan struct{}
	initializeSeen       chan struct{}
	initializedSeen      chan struct{}
	shutdownSeen         chan struct{}
	exitSeen             chan struct{}
	configurationReply   chan json.RawMessage
	configurationChanges chan json.RawMessage
	workspaceChanges     chan json.RawMessage
	capabilities         map[string]any
	progress             bool
	crashAfterReady      bool
	ignoreShutdown       bool
	onceInitialize       sync.Once
	onceInitialized      sync.Once
	onceShutdown         sync.Once
	onceExit             sync.Once
	writeMu              sync.Mutex
}

func newFakeLSPServer() *fakeLSPServer {
	hold := make(chan struct{})
	close(hold)
	return &fakeLSPServer{
		holdInitialize:       hold,
		initializeSeen:       make(chan struct{}),
		initializedSeen:      make(chan struct{}),
		shutdownSeen:         make(chan struct{}),
		exitSeen:             make(chan struct{}),
		configurationReply:   make(chan json.RawMessage, 1),
		configurationChanges: make(chan json.RawMessage, 1),
		workspaceChanges:     make(chan json.RawMessage, 1),
		capabilities:         map[string]any{"hoverProvider": true},
	}
}

func newFakeProcessManager(factory func(int) *fakeLSPServer) *fakeProcessManager {
	return &fakeProcessManager{
		active:        make(map[string]*fakeLSPProcess),
		serverFactory: factory,
		ownedReleased: make(chan struct{}),
	}
}

func (m *fakeProcessManager) BeginOwnedOperation(parent context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(parent)
	return ctx, func() {
		cancel()
		m.releaseOnce.Do(func() { close(m.ownedReleased) })
	}, nil
}

func (m *fakeProcessManager) StartPipedProcess(req process.PipedStartRequest) (*process.PipedProcess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) != 0 {
		m.overlapObserved = true
	}
	m.started++
	id := fmt.Sprintf("lsp-%d", m.started)
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	fakeProcess := &fakeLSPProcess{
		id:           id,
		serverInput:  inputReader,
		serverOutput: outputWriter,
		done:         make(chan struct{}),
	}
	server := m.serverFactory(m.started)
	m.active[id] = fakeProcess
	m.servers = append(m.servers, server)
	go serveFakeLSP(server, fakeProcess, req)
	return &process.PipedProcess{
		ID: id, Stdin: inputWriter, Stdout: outputReader, Done: fakeProcess.done,
	}, nil
}

func (m *fakeProcessManager) StopProcess(_ context.Context, req process.StopProcessRequest) error {
	m.mu.Lock()
	fakeProcess := m.active[req.ProcessID]
	if fakeProcess != nil {
		delete(m.active, req.ProcessID)
		m.stopped++
	}
	m.mu.Unlock()
	if fakeProcess == nil {
		return nil
	}
	fakeProcess.stopOnce.Do(func() {
		_ = fakeProcess.serverInput.Close()
		_ = fakeProcess.serverOutput.Close()
		close(fakeProcess.done)
	})
	return nil
}

func (m *fakeProcessManager) counts() (started, stopped int, overlap bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started, m.stopped, m.overlapObserved
}

func serveFakeLSP(server *fakeLSPServer, fakeProcess *fakeLSPProcess, req process.PipedStartRequest) {
	_ = req
	reader := bufio.NewReader(fakeProcess.serverInput)
	for {
		body, err := protocol.ReadMessage(reader)
		if err != nil {
			return
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &message) != nil {
			return
		}
		switch message.Method {
		case "initialize":
			if server.progress {
				server.write(fakeProcess.serverOutput, map[string]any{
					"jsonrpc": "2.0", "method": "$/progress",
					"params": map[string]any{
						"token": "import", "value": map[string]any{
							"kind": "begin", "title": "Importing Kotlin project", "percentage": 5,
						},
					},
				})
				server.write(fakeProcess.serverOutput, map[string]any{
					"jsonrpc": "2.0", "method": "$/progress",
					"params": map[string]any{
						"token": "import", "value": map[string]any{
							"kind": "report", "message": "Resolving Gradle", "percentage": 42,
						},
					},
				})
			}
			server.onceInitialize.Do(func() { close(server.initializeSeen) })
			<-server.holdInitialize
			if server.progress {
				server.write(fakeProcess.serverOutput, map[string]any{
					"jsonrpc": "2.0", "method": "$/progress",
					"params": map[string]any{
						"token": "import", "value": map[string]any{
							"kind": "end", "message": "Project model loaded",
						},
					},
				})
			}
			server.write(fakeProcess.serverOutput, map[string]any{
				"jsonrpc": "2.0", "id": message.ID,
				"result": map[string]any{"capabilities": server.capabilities},
			})
		case "initialized":
			server.onceInitialized.Do(func() { close(server.initializedSeen) })
			server.write(fakeProcess.serverOutput, map[string]any{
				"jsonrpc": "2.0", "id": 900, "method": "workspace/configuration",
				"params": map[string]any{"items": []map[string]any{{"section": "kotlin"}}},
			})
			if server.crashAfterReady {
				_ = fakeProcess.serverOutput.Close()
				fakeProcess.stopOnce.Do(func() { close(fakeProcess.done) })
				return
			}
		case "shutdown":
			server.onceShutdown.Do(func() { close(server.shutdownSeen) })
			if server.ignoreShutdown {
				continue
			}
			server.write(fakeProcess.serverOutput, map[string]any{
				"jsonrpc": "2.0", "id": message.ID, "result": nil,
			})
		case "exit":
			server.onceExit.Do(func() { close(server.exitSeen) })
		case "workspace/didChangeWorkspaceFolders":
			select {
			case server.workspaceChanges <- append(json.RawMessage(nil), message.Params...):
			default:
			}
		case "workspace/didChangeConfiguration":
			select {
			case server.configurationChanges <- append(json.RawMessage(nil), message.Params...):
			default:
			}
		default:
			if len(message.ID) != 0 && message.Method == "" {
				select {
				case server.configurationReply <- append(json.RawMessage(nil), message.Result...):
				default:
				}
			}
		}
	}
}

func (s *fakeLSPServer) write(writer io.Writer, message any) {
	payload, _ := json.Marshal(message)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(payload))
	_, _ = writer.Write(payload)
}

func newManagerForTest(t *testing.T, factory func(int) *fakeLSPServer) (*Manager, *fakeProcessManager) {
	t.Helper()
	processes := newFakeProcessManager(factory)
	manager := NewManager(Config{
		WorkDir:      "/workspace",
		WorkspaceURI: "file:///workspace",
		WorkspaceFolders: []WorkspaceFolder{
			{URI: "file:///workspace/repo", Name: "repo"},
		},
		OwnerID: "task-1",
	}, processes, &fakeInstaller{binary: "/trusted/kotlin-lsp"}, testLogger())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := manager.Close(ctx); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	return manager, processes
}

type fakeInstaller struct{ binary string }

func (i *fakeInstaller) BinaryPath(string) (string, error) { return i.binary, nil }
func (i *fakeInstaller) StrategyFor(string) (tools.Strategy, error) {
	return nil, fmt.Errorf("not used")
}

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

func waitForPhase(t *testing.T, manager *Manager, language string, phase sharedlsp.Phase) Snapshot {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := manager.Snapshot(language)
		if snapshot.Phase == phase {
			return snapshot
		}
		select {
		case <-deadline.C:
			t.Fatalf("phase = %s, want %s; snapshot=%#v", snapshot.Phase, phase, snapshot)
		case <-ticker.C:
		}
	}
}

func waitForSnapshot(t *testing.T, manager *Manager, language string, ready func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := manager.Snapshot(language)
		if ready(snapshot) {
			return snapshot
		}
		select {
		case <-deadline.C:
			t.Fatalf("snapshot condition not met: %#v", snapshot)
		case <-ticker.C:
		}
	}
}

func TestManagerDeduplicatesGenerationStart(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	request := StartRequest{Language: "kotlin", Generation: 1}

	if _, err := manager.Start(context.Background(), request); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	if _, err := manager.Start(context.Background(), request); err != nil {
		t.Fatalf("retry generation: %v", err)
	}
	snapshot := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	started, _, _ := processes.counts()
	if started != 1 || snapshot.Generation != 1 {
		t.Fatalf("started=%d snapshot=%#v", started, snapshot)
	}
}

func TestManagerCapturesEarlyProgressCapabilitiesAndConfiguration(t *testing.T) {
	server := newFakeLSPServer()
	server.progress = true
	server.holdInitialize = make(chan struct{})
	manager, _ := newManagerForTest(t, func(int) *fakeLSPServer { return server })
	configuration := json.RawMessage(`{"compiler":{"jvm":{"target":"21"}}}`)

	if _, err := manager.Start(context.Background(), StartRequest{
		Language: "kotlin", Generation: 3, Configuration: configuration,
	}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	<-server.initializeSeen
	snapshot := waitForSnapshot(t, manager, "kotlin", func(snapshot Snapshot) bool {
		return snapshot.Phase == sharedlsp.PhaseInitializing && len(snapshot.Work) == 1 &&
			snapshot.Work[0].Message == "Resolving Gradle"
	})
	if snapshot.Activity != ActivityServerWork || len(snapshot.Work) != 1 ||
		snapshot.Work[0].Message != "Resolving Gradle" || snapshot.Work[0].Percentage == nil ||
		*snapshot.Work[0].Percentage != 42 {
		t.Fatalf("early progress snapshot = %#v", snapshot)
	}
	close(server.holdInitialize)
	snapshot = waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if snapshot.Activity != ActivityIdle || len(snapshot.Work) != 0 ||
		snapshot.LastCompletedWork == nil || snapshot.LastCompletedWork.Message != "Project model loaded" {
		t.Fatalf("completed progress snapshot = %#v", snapshot)
	}
	if !json.Valid(snapshot.Capabilities) || !stringContains(snapshot.Capabilities, "hoverProvider") {
		t.Fatalf("capabilities = %s", snapshot.Capabilities)
	}
	select {
	case reply := <-server.configurationReply:
		if !stringContains(reply, "target") {
			t.Fatalf("configuration reply = %s", reply)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("workspace/configuration was not answered")
	}
}

func TestManagerUpdatesConfigurationWithoutRestartingGeneration(t *testing.T) {
	server := newFakeLSPServer()
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return server })
	if _, err := manager.Start(context.Background(), StartRequest{
		Language: "kotlin", Generation: 4, Configuration: json.RawMessage(`{"compiler":{"jvmTarget":"17"}}`),
	}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	select {
	case <-server.configurationChanges:
	default:
	}

	snapshot, err := manager.UpdateConfiguration(context.Background(), sharedlsp.TaskHostConfigurationRequest{
		Language: "kotlin", Generation: 4, Configuration: json.RawMessage(`{"compiler":{"jvmTarget":"21"}}`),
	})
	if err != nil {
		t.Fatalf("update configuration: %v", err)
	}
	select {
	case params := <-server.configurationChanges:
		if !stringContains(params, "21") {
			t.Fatalf("configuration notification = %s", params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("workspace/didChangeConfiguration was not sent")
	}
	started, _, _ := processes.counts()
	if snapshot.Generation != 4 || started != 1 {
		t.Fatalf("snapshot=%#v started=%d, want generation 4 and one process", snapshot, started)
	}
}

func TestManagerRestartReapsBeforeReplacementAndRetriesIdempotently(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation 1: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	request := StartRequest{Language: "kotlin", Generation: 2}
	if _, err := manager.Restart(context.Background(), request); err != nil {
		t.Fatalf("restart generation 2: %v", err)
	}
	if _, err := manager.Restart(context.Background(), request); err != nil {
		t.Fatalf("retry restart generation 2: %v", err)
	}
	snapshot := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	started, stopped, overlap := processes.counts()
	if started != 2 || stopped != 1 || overlap || snapshot.Generation != 2 {
		t.Fatalf("started=%d stopped=%d overlap=%v snapshot=%#v", started, stopped, overlap, snapshot)
	}
}

func TestManagerStopUsesShutdownExitAndCloseReapsRuntime(t *testing.T) {
	servers := make(chan *fakeLSPServer, 2)
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		server := newFakeLSPServer()
		servers <- server
		return server
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	server := <-servers
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if _, err := manager.Stop(context.Background(), StopRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("stop generation: %v", err)
	}
	select {
	case <-server.shutdownSeen:
	default:
		t.Fatal("shutdown request was not observed")
	}
	select {
	case <-server.exitSeen:
	default:
		t.Fatal("exit notification was not observed")
	}
	if snapshot := manager.Snapshot("kotlin"); snapshot.Phase != sharedlsp.PhaseOff {
		t.Fatalf("stopped snapshot = %#v", snapshot)
	}

	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 2}); err != nil {
		t.Fatalf("start generation 2: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	started, stopped, _ := processes.counts()
	if started != 2 || stopped != 2 {
		t.Fatalf("manager close process counts = started %d stopped %d", started, stopped)
	}
	select {
	case <-processes.ownedReleased:
	default:
		t.Fatal("manager close did not release its process-manager ownership")
	}
}

func TestManagerReportsCrashWithoutSpawningReplacement(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		server := newFakeLSPServer()
		server.crashAfterReady = true
		return server
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 5}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	snapshot := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseError)
	started, _, _ := processes.counts()
	if started != 1 || snapshot.ErrorCode != "process_exited" || snapshot.Generation != 5 {
		t.Fatalf("crash snapshot = %#v, started=%d", snapshot, started)
	}
}

func TestManagerConcurrentGenerationActionsStaySingleProcess(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	runConcurrently(t, 12, func() error {
		_, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1})
		return err
	})
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	runConcurrently(t, 12, func() error {
		_, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 2})
		return err
	})
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	runConcurrently(t, 12, func() error {
		_, err := manager.Stop(context.Background(), StopRequest{Language: "kotlin", Generation: 2})
		return err
	})

	started, stopped, overlap := processes.counts()
	if started != 2 || stopped != 2 || overlap {
		t.Fatalf("started=%d stopped=%d overlap=%v", started, stopped, overlap)
	}
	if snapshot := manager.Snapshot("kotlin"); snapshot.Phase != sharedlsp.PhaseOff {
		t.Fatalf("final snapshot = %#v", snapshot)
	}
}

func TestManagerForcesCleanupWhenShutdownDoesNotReply(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		server := newFakeLSPServer()
		server.ignoreShutdown = true
		return server
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := manager.Stop(ctx, StopRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("forced stop: %v", err)
	}
	_, stopped, _ := processes.counts()
	if stopped != 1 {
		t.Fatalf("stopped=%d, want 1", stopped)
	}
}

func TestManagerWatcherDisconnectDoesNotOwnRuntime(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	updates, unsubscribe := manager.Subscribe("kotlin")
	<-updates
	unsubscribe()

	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 {
		t.Fatalf("watcher changed runtime ownership: started=%d stopped=%d", started, stopped)
	}
}

func TestManagerDynamicallyUpdatesWorkspaceFoldersWithoutRestart(t *testing.T) {
	server := newFakeLSPServer()
	server.capabilities = map[string]any{
		"workspace": map[string]any{
			"workspaceFolders": map[string]any{"supported": true, "changeNotifications": true},
		},
	}
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return server })
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	folders := []WorkspaceFolder{{URI: "file:///workspace/repo-2", Name: "repo-2"}}

	result, err := manager.UpdateWorkspaceFolders(folders)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DynamicLanguages) != 1 || result.DynamicLanguages[0] != "kotlin" ||
		len(result.RestartRequiredLanguages) != 0 {
		t.Fatalf("workspace result = %#v", result)
	}
	select {
	case change := <-server.workspaceChanges:
		if !stringContains(change, "repo-2") || !stringContains(change, "repo") {
			t.Fatalf("workspace change = %s", change)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace folder notification not received")
	}
	snapshot := manager.Snapshot("kotlin")
	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 || len(snapshot.WorkspaceFolders) != 1 || snapshot.WorkspaceFolders[0].Name != "repo-2" {
		t.Fatalf("started=%d stopped=%d snapshot=%#v", started, stopped, snapshot)
	}
}

func TestManagerMarksStaticWorkspaceFolderServerForExplicitRestart(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return newFakeLSPServer() })
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	before := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	folders := []WorkspaceFolder{{URI: "file:///workspace/new-repo", Name: "new-repo"}}

	result, err := manager.UpdateWorkspaceFolders(folders)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RestartRequiredLanguages) != 1 || result.RestartRequiredLanguages[0] != "kotlin" ||
		len(result.DynamicLanguages) != 0 {
		t.Fatalf("workspace result = %#v", result)
	}
	after := manager.Snapshot("kotlin")
	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 || len(after.WorkspaceFolders) != 1 ||
		after.WorkspaceFolders[0].URI != before.WorkspaceFolders[0].URI {
		t.Fatalf("static server scope changed: before=%#v after=%#v", before, after)
	}

	if _, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 2}); err != nil {
		t.Fatal(err)
	}
	restarted := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if len(restarted.WorkspaceFolders) != 1 || restarted.WorkspaceFolders[0].Name != "new-repo" {
		t.Fatalf("restarted workspace folders = %#v", restarted.WorkspaceFolders)
	}
}

func TestAttachmentDisconnectDoesNotStopManagerRuntime(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	attachment, err := manager.Attach("kotlin", 1)
	if err != nil {
		t.Fatalf("attach generation: %v", err)
	}
	drainAttached(t, attachment)
	attachment.Close()

	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 {
		t.Fatalf("attachment disconnect changed lifecycle: started=%d stopped=%d", started, stopped)
	}
	if snapshot := manager.Snapshot("kotlin"); snapshot.Phase != sharedlsp.PhaseReady {
		t.Fatalf("snapshot after detach = %#v", snapshot)
	}
}

func TestAttachmentStaleGenerationRejectedAndOldHubClosesOnRestart(t *testing.T) {
	manager, _ := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if _, err := manager.Attach("kotlin", 2); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale attach error = %v, want %v", err, ErrStaleGeneration)
	}
	attachment, err := manager.Attach("kotlin", 1)
	if err != nil {
		t.Fatalf("attach generation: %v", err)
	}
	drainAttached(t, attachment)

	if _, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 2}); err != nil {
		t.Fatalf("restart generation: %v", err)
	}
	for range attachment.Messages() {
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if _, err := manager.Attach("kotlin", 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old generation attach error = %v, want %v", err, ErrStaleGeneration)
	}
}

func runConcurrently(t *testing.T, count int, operation func() error) {
	t.Helper()
	start := make(chan struct{})
	errors := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			errors <- operation()
		}()
	}
	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent operation: %v", err)
		}
	}
}

func stringContains(raw []byte, substring string) bool {
	return bytes.Contains(raw, []byte(substring))
}
