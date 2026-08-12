package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

const (
	gracefulStopTimeout   = 3 * time.Second
	processCleanupTimeout = 5 * time.Second
)

type runtimeConfig struct {
	language      string
	generation    uint64
	configuration json.RawMessage
	workspace     Config
	process       *process.PipedProcess
	manager       *Manager
	ctx           context.Context
}

type runtime struct {
	language   string
	generation uint64
	workspace  Config
	process    *process.PipedProcess
	manager    *Manager
	ctx        context.Context

	configurationMu sync.RWMutex
	configuration   json.RawMessage

	peerMu  sync.RWMutex
	peer    *peer
	peerSet chan struct{}
	done    chan struct{}
	hub     *hub

	initialized           atomic.Bool
	stopping              atomic.Bool
	releaseBackgroundWork func()
}

func newRuntime(cfg runtimeConfig) *runtime {
	runtime := &runtime{
		language:      cfg.language,
		generation:    cfg.generation,
		configuration: append(json.RawMessage(nil), cfg.configuration...),
		workspace:     cfg.workspace,
		process:       cfg.process,
		manager:       cfg.manager,
		ctx:           cfg.ctx,
		peerSet:       make(chan struct{}),
		done:          make(chan struct{}),
	}
	runtime.hub = newHub(cfg.language, cfg.generation, runtime)
	return runtime
}

func (r *runtime) run() (runErr error) {
	defer func() {
		if r.releaseBackgroundWork != nil {
			r.releaseBackgroundWork()
		}
		close(r.done)
	}()
	protocolPeer := newPeer(r.process.Stdin, r.process.Stdout, r.handleRequest, r.handleNotification)
	r.peerMu.Lock()
	r.peer = protocolPeer
	close(r.peerSet)
	r.peerMu.Unlock()
	defer func() {
		protocolPeer.CloseStreams()
		<-protocolPeer.Done()
	}()
	defer r.hub.Close()

	now := time.Now().UTC()
	r.manager.publishForGeneration(r.language, r.generation, func(snapshot *Snapshot) {
		snapshot.Phase = sharedlsp.PhaseInitializing
		snapshot.InitializeStartedAt = &now
	})
	var result struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if err := protocolPeer.Call(r.initializeRequestContext(), "initialize", r.initializeParams(), &result); err != nil {
		return fmt.Errorf("initialize language server: %w", err)
	}
	r.manager.publishForGeneration(r.language, r.generation, func(snapshot *Snapshot) {
		snapshot.Capabilities = append(json.RawMessage(nil), result.Capabilities...)
	})
	if err := protocolPeer.Notify(methodInitialized, map[string]any{}); err != nil {
		return fmt.Errorf("notify initialized: %w", err)
	}
	r.initialized.Store(true)
	configuration := r.configurationSnapshot()
	if len(configuration) != 0 {
		if err := protocolPeer.Notify("workspace/didChangeConfiguration", map[string]any{
			"settings": configuration,
		}); err != nil {
			return fmt.Errorf("send workspace configuration: %w", err)
		}
	}
	readyAt := time.Now().UTC()
	r.manager.publishForGeneration(r.language, r.generation, func(snapshot *Snapshot) {
		snapshot.Phase = sharedlsp.PhaseReady
		snapshot.ReadyAt = &readyAt
		if len(snapshot.Work) == 0 {
			snapshot.Activity = ActivityIdle
		}
	})

	select {
	case <-protocolPeer.Done():
		return protocolPeer.Err()
	case <-r.process.Done:
		return errors.New("language-server process exited")
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

func (r *runtime) initializeRequestContext() context.Context {
	return r.ctx
}

func (r *runtime) initializeParams() map[string]any {
	folders := r.workspace.WorkspaceFolders
	if len(folders) == 0 && r.workspace.WorkspaceURI != "" {
		folders = []WorkspaceFolder{{
			URI:  r.workspace.WorkspaceURI,
			Name: filepath.Base(r.workspace.WorkDir),
		}}
	}
	return map[string]any{
		"processId":             nil,
		"clientInfo":            map[string]string{"name": "kandev-agentctl"},
		"capabilities":          taskHostClientCapabilities(),
		"workDoneToken":         fmt.Sprintf("kandev:%s:%s:%d", r.workspace.OwnerID, r.language, r.generation),
		"rootPath":              r.workspace.WorkDir,
		"rootUri":               r.workspace.WorkspaceURI,
		fieldWorkspaceFolders:   folders,
		"initializationOptions": map[string]any{},
	}
}

func taskHostClientCapabilities() map[string]any {
	return map[string]any{
		"window": map[string]any{"workDoneProgress": true},
		fieldWorkspace: map[string]any{
			"configuration":       true,
			fieldWorkspaceFolders: true,
		},
		fieldTextDocument: map[string]any{
			"synchronization": map[string]any{
				fieldDynamicRegister: false,
				"willSave":           false,
				"didSave":            true,
				"willSaveWaitUntil":  false,
			},
			"completion": map[string]any{
				fieldDynamicRegister: false,
				"completionItem": map[string]any{
					"snippetSupport": true,
				},
			},
			"hover":      map[string]any{fieldDynamicRegister: false},
			"definition": map[string]any{fieldDynamicRegister: false},
			"references": map[string]any{fieldDynamicRegister: false},
			"publishDiagnostics": map[string]any{
				"relatedInformation": true,
			},
		},
	}
}

func (r *runtime) handleRequest(method string, params json.RawMessage) (any, error) {
	switch method {
	case "workspace/configuration":
		return configurationResponse(r.language, r.configurationSnapshot(), params)
	case "window/workDoneProgress/create", "client/registerCapability", "client/unregisterCapability":
		return nil, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
	}
}

func (r *runtime) configurationSnapshot() json.RawMessage {
	r.configurationMu.RLock()
	defer r.configurationMu.RUnlock()
	return append(json.RawMessage(nil), r.configuration...)
}

func (r *runtime) updateConfiguration(configuration json.RawMessage, notify bool) error {
	r.configurationMu.Lock()
	r.configuration = append(json.RawMessage(nil), configuration...)
	r.configurationMu.Unlock()
	if !notify {
		return nil
	}
	params, err := json.Marshal(map[string]any{
		"settings": append(json.RawMessage(nil), configuration...),
	})
	if err != nil {
		return fmt.Errorf("encode workspace configuration: %w", err)
	}
	return r.Notify("workspace/didChangeConfiguration", params)
}

func (r *runtime) handleNotification(method string, params json.RawMessage) {
	switch method {
	case methodProgress:
		r.manager.applyProgress(r.language, r.generation, params)
	case "textDocument/publishDiagnostics":
		r.manager.applyDiagnostics(r.language, r.generation, params)
		r.hub.Broadcast(method, params)
	default:
		r.hub.Broadcast(method, params)
	}
}

func (r *runtime) Request(
	ctx context.Context,
	method string,
	params json.RawMessage,
) (json.RawMessage, error) {
	select {
	case <-r.peerSet:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	r.peerMu.RLock()
	protocolPeer := r.peer
	r.peerMu.RUnlock()
	if protocolPeer == nil {
		return nil, ErrRuntimeNotReady
	}
	return protocolPeer.CallRaw(ctx, method, params)
}

func (r *runtime) Notify(method string, params json.RawMessage) error {
	select {
	case <-r.peerSet:
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
	r.peerMu.RLock()
	protocolPeer := r.peer
	r.peerMu.RUnlock()
	if protocolPeer == nil {
		return ErrRuntimeNotReady
	}
	return protocolPeer.Notify(method, params)
}

func (r *runtime) requestShutdown(parent context.Context) {
	select {
	case <-r.peerSet:
	case <-parent.Done():
		return
	}
	r.peerMu.RLock()
	protocolPeer := r.peer
	r.peerMu.RUnlock()
	if protocolPeer == nil {
		return
	}
	if !r.initialized.Load() {
		protocolPeer.CloseStreams()
		return
	}
	shutdownCtx := parent
	var cancel context.CancelFunc
	if _, hasDeadline := parent.Deadline(); !hasDeadline {
		shutdownCtx, cancel = context.WithTimeout(parent, gracefulStopTimeout)
		defer cancel()
	}
	_ = protocolPeer.Call(shutdownCtx, methodShutdown, nil, nil)
	_ = protocolPeer.Notify(methodExit, nil)
	_ = protocolPeer.CloseInput()
}

func (r *runtime) closeStreams() {
	select {
	case <-r.peerSet:
		r.peerMu.RLock()
		protocolPeer := r.peer
		r.peerMu.RUnlock()
		if protocolPeer != nil {
			protocolPeer.CloseStreams()
			return
		}
	default:
	}
	_ = r.process.Stdin.Close()
	_ = r.process.Stdout.Close()
}

func configurationResponse(language string, configuration, params json.RawMessage) ([]any, error) {
	var request struct {
		Items []struct {
			Section string `json:"section"`
		} `json:"items"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, fmt.Errorf("decode workspace/configuration request: %w", err)
	}
	var settings any = map[string]any{}
	if len(configuration) != 0 {
		if err := json.Unmarshal(configuration, &settings); err != nil {
			return nil, err
		}
	}
	if len(request.Items) == 0 {
		return []any{settings}, nil
	}
	result := make([]any, len(request.Items))
	for index, item := range request.Items {
		section := item.Section
		if section == language {
			result[index] = settings
			continue
		}
		prefix := language + "."
		section = strings.TrimPrefix(section, prefix)
		result[index] = configurationSection(settings, section)
	}
	return result, nil
}

func configurationSection(settings any, section string) any {
	if section == "" {
		return settings
	}
	current := settings
	for _, part := range splitConfigurationSection(section) {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[part]
		if !ok {
			return nil
		}
	}
	return current
}

func splitConfigurationSection(section string) []string {
	var parts []string
	start := 0
	for index := range section {
		if section[index] == '.' {
			parts = append(parts, section[start:index])
			start = index + 1
		}
	}
	return append(parts, section[start:])
}
