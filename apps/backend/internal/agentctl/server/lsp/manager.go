package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/lsp/installer"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

var (
	ErrManagerClosed     = errors.New("task-host LSP manager is closed")
	ErrStaleGeneration   = errors.New("task-host LSP generation is stale")
	ErrTaskRuntimeActive = errors.New("task-host LSP task runtime is still active")
	ErrTaskStatePurging  = errors.New("task-host LSP task state is being purged")
)

type ProcessManager interface {
	StartPipedProcess(req process.PipedStartRequest) (*process.PipedProcess, error)
	StopProcess(ctx context.Context, req process.StopProcessRequest) error
	BeginOwnedOperation(parent context.Context) (context.Context, func(), error)
}

type Installer interface {
	BinaryPath(language string) (string, error)
	StrategyFor(language string) (tools.Strategy, error)
}

type backgroundWorkOwner interface {
	BeginBackgroundWork() func()
}

type Manager struct {
	cfg       Config
	processes ProcessManager
	installer Installer
	logger    *logger.Logger

	mu           sync.RWMutex
	slots        map[string]*languageSlot
	snapshots    map[string]Snapshot
	subscribers  map[string]map[uint64]chan Snapshot
	nextSubID    uint64
	incarnation  string
	startedAt    time.Time
	closed       bool
	closeErr     error
	taskConfigs  map[string]Config
	purgingTasks map[string]struct{}

	lifetimeMu      sync.Mutex
	lifetimeCtx     context.Context
	lifetimeCancel  context.CancelFunc
	lifetimeRelease func()
	lifetimeDone    chan struct{}
	closeOnce       sync.Once
}

func NewManager(cfg Config, processes ProcessManager, installerRegistry Installer, log *logger.Logger) *Manager {
	return &Manager{
		cfg:          cfg,
		processes:    processes,
		installer:    installerRegistry,
		logger:       log,
		slots:        make(map[string]*languageSlot),
		snapshots:    make(map[string]Snapshot),
		subscribers:  make(map[string]map[uint64]chan Snapshot),
		taskConfigs:  make(map[string]Config),
		purgingTasks: make(map[string]struct{}),
		incarnation:  uuid.NewString(),
		startedAt:    time.Now().UTC(),
	}
}

func (m *Manager) Snapshot(language string) Snapshot {
	return m.SnapshotForTask(m.cfg.OwnerID, language)
}

func (m *Manager) SnapshotForTask(taskID, language string) Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneSnapshot(m.snapshotLocked(normalizeTaskID(taskID, m.cfg.OwnerID), language))
}

func (m *Manager) Subscribe(language string) (<-chan Snapshot, func()) {
	return m.SubscribeForTask(m.cfg.OwnerID, language)
}

func (m *Manager) SubscribeForTask(taskID, language string) (<-chan Snapshot, func()) {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	key := taskLanguageRuntimeKey(taskID, language)
	updates := make(chan Snapshot, 1)
	m.mu.Lock()
	if m.closed {
		close(updates)
		m.mu.Unlock()
		return updates, func() {}
	}
	if _, purging := m.purgingTasks[taskID]; purging {
		close(updates)
		m.mu.Unlock()
		return updates, func() {}
	}
	m.nextSubID++
	id := m.nextSubID
	if m.subscribers[key] == nil {
		m.subscribers[key] = make(map[uint64]chan Snapshot)
	}
	m.subscribers[key][id] = updates
	updates <- cloneSnapshot(m.snapshotLocked(taskID, language))
	m.mu.Unlock()

	var once sync.Once
	return updates, func() {
		once.Do(func() {
			m.mu.Lock()
			if subscriber, ok := m.subscribers[key][id]; ok {
				delete(m.subscribers[key], id)
				close(subscriber)
			}
			m.mu.Unlock()
		})
	}
}

func (m *Manager) Attach(language string, generation uint64) (*Attachment, error) {
	return m.AttachForTask(m.cfg.OwnerID, language, generation)
}

func (m *Manager) AttachForTask(taskID, language string, generation uint64) (*Attachment, error) {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	if !installer.IsSupported(language) {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}
	slot, err := m.slotForTask(taskID, language)
	if err != nil {
		return nil, err
	}
	slot.opMu.Lock()
	defer slot.opMu.Unlock()
	if slot.runtime == nil {
		return nil, ErrRuntimeNotReady
	}
	if generation != 0 && generation != slot.runtime.generation {
		return nil, ErrStaleGeneration
	}
	snapshot := m.SnapshotForTask(taskID, language)
	if snapshot.Phase != sharedlsp.PhaseReady || snapshot.Generation != slot.runtime.generation {
		return nil, ErrRuntimeNotReady
	}
	return slot.runtime.hub.Attach(snapshot), nil
}

func (m *Manager) Start(_ context.Context, request StartRequest) (Snapshot, error) {
	return m.start(request)
}

func (m *Manager) Restart(_ context.Context, request StartRequest) (Snapshot, error) {
	return m.start(request)
}

func (m *Manager) UpdateConfiguration(_ context.Context, request ConfigurationRequest) (Snapshot, error) {
	request.TaskID = normalizeTaskID(request.TaskID, m.cfg.OwnerID)
	if err := validateConfigurationRequest(request); err != nil {
		return Snapshot{}, err
	}
	slot, err := m.slotForTask(request.TaskID, request.Language)
	if err != nil {
		return Snapshot{}, err
	}
	slot.opMu.Lock()
	defer slot.opMu.Unlock()
	if slot.runtime == nil {
		return m.SnapshotForTask(request.TaskID, request.Language), ErrRuntimeNotReady
	}
	if request.Generation != slot.runtime.generation {
		return m.SnapshotForTask(request.TaskID, request.Language), ErrStaleGeneration
	}
	snapshot := m.SnapshotForTask(request.TaskID, request.Language)
	notify := snapshot.Phase == sharedlsp.PhaseReady
	if err := slot.runtime.updateConfiguration(request.Configuration, notify); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (m *Manager) start(request StartRequest) (Snapshot, error) {
	request.TaskID = normalizeTaskID(request.TaskID, m.cfg.OwnerID)
	if err := validateStartRequest(request); err != nil {
		return Snapshot{}, err
	}
	lifetimeCtx, err := m.ensureLifetime()
	if err != nil {
		return Snapshot{}, err
	}
	slot, err := m.slotForTask(request.TaskID, request.Language)
	if err != nil {
		return Snapshot{}, err
	}

	operationCtx, unlock := slot.lockStartOperation(lifetimeCtx, request.Generation)
	defer unlock()
	if err := m.checkOpen(); err != nil {
		return Snapshot{}, err
	}
	if err := operationCtx.Err(); err != nil {
		return m.SnapshotForTask(request.TaskID, request.Language), err
	}
	if request.Generation < slot.lastGeneration {
		return m.SnapshotForTask(request.TaskID, request.Language), ErrStaleGeneration
	}
	if request.Generation == slot.lastGeneration {
		return m.SnapshotForTask(request.TaskID, request.Language), nil
	}
	if slot.runtime != nil {
		m.publishTaskPhase(request.TaskID, request.Language, slot.runtime.generation, sharedlsp.PhaseStopping)
		if err := m.stopRuntime(context.Background(), slot.runtime); err != nil {
			return m.publishTaskError(
				request.TaskID, request.Language, slot.runtime.generation, "replacement_cleanup_failed", err,
			), err
		}
		slot.runtime = nil
	}
	// Accept the replacement generation only after the previous process tree
	// is proven gone. A failed cleanup remains retryable with the same request.
	slot.lastGeneration = request.Generation

	workspace := m.configSnapshot(request.TaskID)
	m.publishForTaskGeneration(request.TaskID, request.Language, request.Generation, func(snapshot *Snapshot) {
		resetRuntimeSnapshot(snapshot)
		snapshot.Phase = sharedlsp.PhaseStarting
		snapshot.WorkspacePath = workspace.WorkDir
		snapshot.WorkspaceURI = workspace.WorkspaceURI
		snapshot.WorkspaceFolders = append([]WorkspaceFolder(nil), workspace.WorkspaceFolders...)
	})
	binaryPath, err := m.resolveBinary(operationCtx, request)
	if err != nil {
		return m.publishTaskError(request.TaskID, request.Language, request.Generation, classifyStartError(err), err), err
	}

	server, err := m.startProcess(request, binaryPath, workspace)
	if err != nil {
		return m.publishTaskError(request.TaskID, request.Language, request.Generation, "process_start_failed", err), err
	}
	runtime := newRuntime(runtimeConfig{
		taskID:        request.TaskID,
		language:      request.Language,
		generation:    request.Generation,
		configuration: request.Configuration,
		workspace:     workspace,
		process:       server,
		manager:       m,
		ctx:           lifetimeCtx,
	})
	if owner, ok := m.processes.(backgroundWorkOwner); ok {
		runtime.releaseBackgroundWork = owner.BeginBackgroundWork()
	}
	slot.runtime = runtime
	now := time.Now().UTC()
	m.publishForTaskGeneration(request.TaskID, request.Language, request.Generation, func(snapshot *Snapshot) {
		snapshot.Phase = sharedlsp.PhaseProcessStarted
		snapshot.ProcessStartedAt = &now
	})
	go m.runRuntime(slot, runtime)
	return m.SnapshotForTask(request.TaskID, request.Language), nil
}

// UpdateWorkspaceFolders applies one task-level root change to every live
// language. Capable servers receive the standard dynamic notification; other
// generations keep their old scope and are reported restart-required.
func (m *Manager) UpdateWorkspaceFolders(folders []WorkspaceFolder) (sharedlsp.WorkspaceUpdateResult, error) {
	return m.UpdateWorkspaceFoldersForTask(m.cfg.OwnerID, folders)
}

func (m *Manager) UpdateWorkspaceFoldersForTask(
	taskID string,
	folders []WorkspaceFolder,
) (sharedlsp.WorkspaceUpdateResult, error) {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	folders = normalizeWorkspaceFolders(folders)
	m.mu.Lock()
	if _, purging := m.purgingTasks[taskID]; purging {
		m.mu.Unlock()
		return sharedlsp.WorkspaceUpdateResult{}, ErrTaskStatePurging
	}
	taskConfig := m.cfg
	if configured, ok := m.taskConfigs[taskID]; ok {
		taskConfig = configured
	}
	taskConfig.OwnerID = taskID
	taskConfig.WorkspaceFolders = append([]WorkspaceFolder(nil), folders...)
	m.taskConfigs[taskID] = taskConfig
	m.mu.Unlock()
	return m.applyWorkspaceFoldersForTask(taskID, folders)
}

// UpdateWorkspaceForTask records a complete, backend-authorized workspace
// projection without rebinding the physical task-host tracker graph.
func (m *Manager) UpdateWorkspaceForTask(
	taskID, workspacePath string,
	workspaceRoots []string,
) (sharedlsp.WorkspaceUpdateResult, error) {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	workspacePath = filepath.Clean(workspacePath)
	if !filepath.IsAbs(workspacePath) {
		return sharedlsp.WorkspaceUpdateResult{}, errors.New("task LSP workspace_path must be absolute")
	}
	workspaceRoots = normalizeWorkspaceRoots(workspaceRoots)
	folders := WorkspaceFoldersAtRoots(workspacePath, workspaceRoots)
	if len(folders) == 0 {
		return sharedlsp.WorkspaceUpdateResult{}, errors.New("task LSP workspace has no valid roots")
	}
	m.mu.Lock()
	if _, purging := m.purgingTasks[taskID]; purging {
		m.mu.Unlock()
		return sharedlsp.WorkspaceUpdateResult{}, ErrTaskStatePurging
	}
	taskConfig := m.cfg
	if configured, ok := m.taskConfigs[taskID]; ok {
		taskConfig = configured
	}
	taskConfig.OwnerID = taskID
	taskConfig.WorkDir = workspacePath
	taskConfig.WorkspaceURI = WorkspaceFileURI(workspacePath)
	taskConfig.WorkspaceFolders = append([]WorkspaceFolder(nil), folders...)
	taskConfig.DiscoveryRoots = append([]string(nil), workspaceRoots...)
	if len(taskConfig.DiscoveryRoots) == 0 {
		taskConfig.DiscoveryRoots = []string{workspacePath}
	}
	m.taskConfigs[taskID] = taskConfig
	m.mu.Unlock()
	return m.applyWorkspaceFoldersForTask(taskID, folders)
}

func (m *Manager) applyWorkspaceFoldersForTask(
	taskID string,
	folders []WorkspaceFolder,
) (sharedlsp.WorkspaceUpdateResult, error) {
	result := sharedlsp.WorkspaceUpdateResult{WorkspaceFolders: append([]WorkspaceFolder(nil), folders...)}
	m.mu.RLock()
	languages := make([]string, 0, len(m.slots))
	for key := range m.slots {
		keyTaskID, language := splitTaskLanguageRuntimeKey(key)
		if keyTaskID == taskID {
			languages = append(languages, language)
		}
	}
	m.mu.RUnlock()
	sort.Strings(languages)

	var updateErrors []error
	for _, language := range languages {
		slot, err := m.slotForTask(taskID, language)
		if err != nil {
			updateErrors = append(updateErrors, err)
			continue
		}
		slot.opMu.Lock()
		current := slot.runtime
		if current == nil {
			slot.opMu.Unlock()
			continue
		}
		snapshot := m.SnapshotForTask(taskID, language)
		if snapshot.Phase != sharedlsp.PhaseReady || !supportsWorkspaceFolderChanges(snapshot.Capabilities) {
			result.RestartRequiredLanguages = append(result.RestartRequiredLanguages, language)
			slot.opMu.Unlock()
			continue
		}
		added, removed := workspaceFolderDiff(current.workspace.WorkspaceFolders, folders)
		if len(added) == 0 && len(removed) == 0 &&
			!workspaceFoldersEqual(current.workspace.WorkspaceFolders, folders) {
			// LSP folder-change notifications describe set membership only. A
			// pure reorder (or rename at the same URI) cannot be represented
			// honestly, so preserve the live generation's scope and require one
			// deliberate restart to adopt the new ordered roots.
			result.RestartRequiredLanguages = append(result.RestartRequiredLanguages, language)
			slot.opMu.Unlock()
			continue
		}
		if len(added) != 0 || len(removed) != 0 {
			params, marshalErr := json.Marshal(map[string]any{
				"event": map[string]any{"added": added, "removed": removed},
			})
			if marshalErr != nil {
				result.RestartRequiredLanguages = append(result.RestartRequiredLanguages, language)
				updateErrors = append(updateErrors, marshalErr)
				slot.opMu.Unlock()
				continue
			}
			if notifyErr := current.Notify("workspace/didChangeWorkspaceFolders", params); notifyErr != nil {
				result.RestartRequiredLanguages = append(result.RestartRequiredLanguages, language)
				updateErrors = append(updateErrors, fmt.Errorf("update %s workspace folders: %w", language, notifyErr))
				slot.opMu.Unlock()
				continue
			}
		}
		current.workspace.WorkspaceFolders = append([]WorkspaceFolder(nil), folders...)
		m.publishForTaskGeneration(taskID, language, current.generation, func(next *Snapshot) {
			next.WorkspaceFolders = append([]WorkspaceFolder(nil), folders...)
		})
		result.DynamicLanguages = append(result.DynamicLanguages, language)
		slot.opMu.Unlock()
	}
	return result, errors.Join(updateErrors...)
}

func (m *Manager) DiscoveryRootsForTask(taskID string) []string {
	config := m.configSnapshot(taskID)
	if len(config.DiscoveryRoots) == 0 && config.WorkDir != "" {
		return []string{config.WorkDir}
	}
	return append([]string(nil), config.DiscoveryRoots...)
}

func (m *Manager) configSnapshot(taskID string) Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := m.cfg
	if configured, ok := m.taskConfigs[taskID]; ok {
		result = configured
	}
	result.OwnerID = normalizeTaskID(taskID, m.cfg.OwnerID)
	result.WorkspaceFolders = append([]WorkspaceFolder(nil), result.WorkspaceFolders...)
	result.DiscoveryRoots = append([]string(nil), result.DiscoveryRoots...)
	return result
}

func normalizeWorkspaceRoots(roots []string) []string {
	result := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		root = filepath.Clean(root)
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		result = append(result, root)
	}
	return result
}

func normalizeWorkspaceFolders(folders []WorkspaceFolder) []WorkspaceFolder {
	result := make([]WorkspaceFolder, 0, len(folders))
	seen := make(map[string]bool, len(folders))
	for _, folder := range folders {
		if folder.URI == "" || seen[folder.URI] {
			continue
		}
		seen[folder.URI] = true
		result = append(result, folder)
	}
	return result
}

func workspaceFoldersEqual(left, right []WorkspaceFolder) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func supportsWorkspaceFolderChanges(capabilities json.RawMessage) bool {
	var decoded struct {
		Workspace struct {
			WorkspaceFolders struct {
				Supported           bool            `json:"supported"`
				ChangeNotifications json.RawMessage `json:"changeNotifications"`
			} `json:"workspaceFolders"`
		} `json:"workspace"`
	}
	if json.Unmarshal(capabilities, &decoded) != nil || !decoded.Workspace.WorkspaceFolders.Supported {
		return false
	}
	raw := decoded.Workspace.WorkspaceFolders.ChangeNotifications
	return len(raw) != 0 && string(raw) != "false" && string(raw) != jsonNull && string(raw) != `""`
}

func workspaceFolderDiff(before, after []WorkspaceFolder) (added, removed []WorkspaceFolder) {
	beforeByURI := make(map[string]WorkspaceFolder, len(before))
	afterByURI := make(map[string]WorkspaceFolder, len(after))
	for _, folder := range before {
		beforeByURI[folder.URI] = folder
	}
	for _, folder := range after {
		afterByURI[folder.URI] = folder
		if _, exists := beforeByURI[folder.URI]; !exists {
			added = append(added, folder)
		}
	}
	for _, folder := range before {
		if _, exists := afterByURI[folder.URI]; !exists {
			removed = append(removed, folder)
		}
	}
	return added, removed
}

func (m *Manager) Stop(ctx context.Context, request StopRequest) (Snapshot, error) {
	request.TaskID = normalizeTaskID(request.TaskID, m.cfg.OwnerID)
	if !installer.IsSupported(request.Language) {
		return Snapshot{}, fmt.Errorf("unsupported language: %s", request.Language)
	}
	slot, err := m.slotForTask(request.TaskID, request.Language)
	if err != nil {
		if errors.Is(err, ErrManagerClosed) {
			return m.SnapshotForTask(request.TaskID, request.Language), nil
		}
		return Snapshot{}, err
	}
	slot.lockAfterCancelingStart(request.Generation)
	defer slot.opMu.Unlock()
	if request.Generation != 0 && request.Generation < slot.lastGeneration {
		return m.SnapshotForTask(request.TaskID, request.Language), ErrStaleGeneration
	}
	if slot.runtime == nil {
		return m.publishTaskOff(request.TaskID, request.Language, slot.lastGeneration), nil
	}
	m.publishTaskPhase(request.TaskID, request.Language, slot.runtime.generation, sharedlsp.PhaseStopping)
	if err := m.stopRuntime(ctx, slot.runtime); err != nil {
		return m.publishTaskError(request.TaskID, request.Language, slot.runtime.generation, "process_stop_failed", err), err
	}
	generation := slot.runtime.generation
	slot.runtime = nil
	return m.publishTaskOff(request.TaskID, request.Language, generation), nil
}

func (m *Manager) Close(ctx context.Context) error {
	m.closeOnce.Do(func() {
		m.closeErr = m.closeAll(ctx)
	})

	m.lifetimeMu.Lock()
	done := m.lifetimeDone
	m.lifetimeMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return errors.Join(m.closeErr, ctx.Err())
		}
	}
	return m.closeErr
}

func (m *Manager) ensureLifetime() (context.Context, error) {
	m.lifetimeMu.Lock()
	defer m.lifetimeMu.Unlock()
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	if m.lifetimeCtx != nil {
		return m.lifetimeCtx, nil
	}
	ownedCtx, release, err := m.processes.BeginOwnedOperation(context.Background())
	if err != nil {
		return nil, err
	}
	lifetimeCtx, cancel := context.WithCancel(ownedCtx)
	done := make(chan struct{})
	m.lifetimeCtx = lifetimeCtx
	m.lifetimeCancel = cancel
	m.lifetimeRelease = release
	m.lifetimeDone = done
	go func() {
		<-ownedCtx.Done()
		m.closeOnce.Do(func() {
			m.closeErr = m.closeAll(context.Background())
		})
		close(done)
	}()
	return lifetimeCtx, nil
}

func (m *Manager) closeAll(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	for language, subscribers := range m.subscribers {
		for id, subscriber := range subscribers {
			delete(subscribers, id)
			close(subscriber)
		}
		delete(m.subscribers, language)
	}
	slots := make([]*languageSlot, 0, len(m.slots))
	for _, slot := range m.slots {
		slots = append(slots, slot)
	}
	m.mu.Unlock()

	m.lifetimeMu.Lock()
	cancel := m.lifetimeCancel
	release := m.lifetimeRelease
	m.lifetimeMu.Unlock()
	if cancel != nil {
		cancel()
	}

	var stopErrors []error
	for _, slot := range slots {
		slot.opMu.Lock()
		if slot.runtime != nil {
			m.publishTaskPhase(
				slot.runtime.taskID, slot.runtime.language, slot.runtime.generation, sharedlsp.PhaseStopping,
			)
			if err := m.stopRuntime(ctx, slot.runtime); err != nil {
				stopErrors = append(stopErrors, err)
				m.publishTaskError(
					slot.runtime.taskID,
					slot.runtime.language,
					slot.runtime.generation,
					"process_cleanup_failed",
					err,
				)
				slot.opMu.Unlock()
				continue
			}
			m.publishTaskOff(slot.runtime.taskID, slot.runtime.language, slot.runtime.generation)
			slot.runtime = nil
		}
		slot.opMu.Unlock()
	}
	if release != nil {
		release()
	}
	return errors.Join(stopErrors...)
}

func (m *Manager) resolveBinary(ctx context.Context, request StartRequest) (string, error) {
	binaryPath, err := m.installer.BinaryPath(request.Language)
	if err == nil {
		return binaryPath, nil
	}
	if !request.AutoInstall || !installer.CanAutoInstall(request.Language) {
		return "", err
	}
	m.publishTaskPhase(request.TaskID, request.Language, request.Generation, sharedlsp.PhaseInstalling)
	return sharedInstallCoordinator.run(ctx, installMutationKey(request.Language), func() (string, error) {
		// Another task/language may have populated the shared cache while this
		// generation waited for its mutation key.
		if resolved, resolveErr := m.installer.BinaryPath(request.Language); resolveErr == nil {
			return resolved, nil
		}
		strategy, strategyErr := m.installer.StrategyFor(request.Language)
		if strategyErr != nil {
			return "", strategyErr
		}
		result, installErr := strategy.Install(ctx)
		if installErr != nil {
			return "", installErr
		}
		return result.BinaryPath, nil
	})
}

func (m *Manager) startProcess(request StartRequest, binaryPath string, workspace Config) (*process.PipedProcess, error) {
	_, args := installer.LspCommand(request.Language)
	return m.processes.StartPipedProcess(process.PipedStartRequest{
		SessionID:  workspace.OwnerID,
		Kind:       types.ProcessKindCustom,
		ScriptName: "lsp-" + request.Language,
		Command:    binaryPath,
		Args:       args,
		WorkingDir: workspace.WorkDir,
		Env: map[string]string{
			"KANDEV_LSP_GENERATION": fmt.Sprintf("%d", request.Generation),
		},
	})
}

func (m *Manager) runRuntime(slot *languageSlot, current *runtime) {
	err := current.run()
	slot.opMu.Lock()
	defer slot.opMu.Unlock()
	if slot.runtime != current {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), processCleanupTimeout)
	defer cancel()
	cleanupErr := m.processes.StopProcess(
		cleanupCtx,
		process.StopProcessRequest{ProcessID: current.process.ID},
	)
	if errors.Is(cleanupErr, process.ErrProcessNotFound) {
		cleanupErr = nil
	}
	cleanupErr = errors.Join(cleanupErr, waitForRuntimeCleanup(cleanupCtx, current))
	if cleanupErr != nil {
		if !current.stopping.Load() && !m.isClosed() {
			m.publishTaskError(
				current.taskID,
				current.language,
				current.generation,
				"process_cleanup_failed",
				errors.Join(err, cleanupErr),
			)
		}
		return
	}
	slot.runtime = nil
	if current.stopping.Load() {
		// Stop may return before process cleanup can be proven. If the runtime
		// goroutine subsequently completes that cleanup, publish the terminal
		// evidence so the backend can release task/language capacity.
		m.publishTaskOff(current.taskID, current.language, current.generation)
		return
	}
	if m.isClosed() {
		return
	}
	if err == nil {
		err = errors.New("language server exited")
	}
	m.publishTaskError(current.taskID, current.language, current.generation, "process_exited", err)
}

func (m *Manager) stopRuntime(ctx context.Context, current *runtime) error {
	current.stopping.Store(true)
	current.hub.Close()
	current.requestShutdown(ctx)
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	stopErr := m.processes.StopProcess(cleanupCtx, process.StopProcessRequest{ProcessID: current.process.ID})
	if errors.Is(stopErr, process.ErrProcessNotFound) {
		stopErr = nil
	}
	current.closeStreams()
	stopErr = errors.Join(stopErr, waitForRuntimeCleanup(cleanupCtx, current))
	return stopErr
}

func waitForRuntimeCleanup(ctx context.Context, current *runtime) error {
	select {
	case <-current.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return current.process.WaitCleanup(ctx)
}

func cleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) > 0 {
		return context.WithDeadline(context.Background(), deadline)
	}
	return context.WithTimeout(context.Background(), processCleanupTimeout)
}

func validateStartRequest(request StartRequest) error {
	if !installer.IsSupported(request.Language) {
		return fmt.Errorf("unsupported language: %s", request.Language)
	}
	if request.Generation == 0 {
		return errors.New("generation must be greater than zero")
	}
	if len(request.Configuration) != 0 && !json.Valid(request.Configuration) {
		return errors.New("configuration must be valid JSON")
	}
	return nil
}

func validateConfigurationRequest(request ConfigurationRequest) error {
	if !installer.IsSupported(request.Language) {
		return fmt.Errorf("unsupported language: %s", request.Language)
	}
	if request.Generation == 0 {
		return errors.New("generation must be greater than zero")
	}
	if !json.Valid(request.Configuration) {
		return errors.New("configuration must be valid JSON")
	}
	return nil
}

func classifyStartError(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "start_canceled"
	}
	return "binary_unavailable"
}

func (m *Manager) slotFor(language string) (*languageSlot, error) {
	return m.slotForTask(m.cfg.OwnerID, language)
}

func (m *Manager) slotForTask(taskID, language string) (*languageSlot, error) {
	taskID = normalizeTaskID(taskID, m.cfg.OwnerID)
	key := taskLanguageRuntimeKey(taskID, language)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
	if _, purging := m.purgingTasks[taskID]; purging {
		return nil, ErrTaskStatePurging
	}
	if m.slots[key] == nil {
		m.slots[key] = &languageSlot{}
	}
	return m.slots[key], nil
}

func (m *Manager) checkOpen() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrManagerClosed
	}
	return nil
}

func (m *Manager) isClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}
