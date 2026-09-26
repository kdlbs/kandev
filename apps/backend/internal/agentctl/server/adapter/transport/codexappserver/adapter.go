// Package codexappserver translates Codex's native app-server protocol into
// Kandev's agent stream contract.
package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
	"go.uber.org/zap"
)

const (
	serverName             = "codex"
	serverVersion          = protocol.SupportedCodexVersion
	maxQueuedUpdates       = 128
	maxCompletedTurns      = 128
	backgroundPollInterval = 2 * time.Second
)

var errNoActiveThread = errors.New("codex app-server thread is not active")

type serverRequestDisposition uint8

const (
	serverRequestUnknown serverRequestDisposition = iota
	serverRequestRejected
	serverRequestSupported
)

var serverRequestDispositions = map[string]serverRequestDisposition{
	protocol.ServerRequestCommandExecutionApproval: serverRequestSupported,
	protocol.ServerRequestFileChangeApproval:       serverRequestSupported,
	protocol.ServerRequestToolUserInput:            serverRequestSupported,
	protocol.ServerRequestMCPElicitation:           serverRequestRejected,
	protocol.ServerRequestPermissionsApproval:      serverRequestRejected,
	protocol.ServerRequestDynamicToolCall:          serverRequestRejected,
	protocol.ServerRequestAuthTokensRefresh:        serverRequestRejected,
	protocol.ServerRequestAttestationGenerate:      serverRequestRejected,
	protocol.ServerRequestApplyPatchApproval:       serverRequestRejected,
	protocol.ServerRequestExecCommandApproval:      serverRequestRejected,
}

// AgentInfo is the connected provider identity. The adapter package wraps this
// value at its boundary to avoid a transport-to-factory import cycle.
type AgentInfo struct {
	Name    string
	Version string
}

type childBinding struct {
	toolCallID     string
	parentThreadID string
	generation     uint64
	description    string
	subagentType   string
	model          string
}

// Adapter owns one app-server connection and one Codex thread at a time.
// Process ownership remains with agentctl's process manager.
type Adapter struct {
	cfg *shared.Config
	log *logger.Logger

	mu                       sync.RWMutex
	client                   *protocol.Client
	info                     *AgentInfo
	threadID                 string
	turnID                   string
	promptPending            bool
	modelID                  string
	activeGeneration         uint64
	turnSequence             uint64
	models                   []streams.SessionModelInfo
	permission               agenttypes.PermissionHandler
	userInputRequest         agenttypes.UserInputRequestHandler
	closed                   bool
	completed                map[string]struct{}
	children                 map[string]childBinding
	childStatuses            map[string]string
	earlyChildActivities     map[string]string
	backgrounds              map[string]protocol.BackgroundTerminal
	backgroundSnapshotLoaded bool
	backgroundCancel         context.CancelFunc
	latestTokenTotals        map[string]protocol.TokenUsageBreakdown
	latestContextWindows     map[string]int64
	turnTokenBaselines       map[string]protocol.TokenUsageBreakdown
	turnHasTokenBaseline     map[string]bool
	turnModels               map[string]string
	turnGenerations          map[string]uint64
	turnResponseObserved     map[string]bool
	turnFallbackSelected     map[string]bool
	completedProviderTurns   map[string]bool
	updates                  chan streams.AgentEvent
	updatesDone              chan struct{}
	updatesMu                sync.RWMutex
	updatesOnce              sync.Once
}

func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	if cfg == nil {
		cfg = &shared.Config{}
	}
	return &Adapter{
		cfg:                    cfg,
		log:                    log,
		completed:              make(map[string]struct{}),
		children:               make(map[string]childBinding),
		childStatuses:          make(map[string]string),
		earlyChildActivities:   make(map[string]string),
		backgrounds:            make(map[string]protocol.BackgroundTerminal),
		latestTokenTotals:      make(map[string]protocol.TokenUsageBreakdown),
		latestContextWindows:   make(map[string]int64),
		turnTokenBaselines:     make(map[string]protocol.TokenUsageBreakdown),
		turnHasTokenBaseline:   make(map[string]bool),
		turnModels:             make(map[string]string),
		turnGenerations:        make(map[string]uint64),
		turnResponseObserved:   make(map[string]bool),
		turnFallbackSelected:   make(map[string]bool),
		completedProviderTurns: make(map[string]bool),
		updates:                make(chan streams.AgentEvent, maxQueuedUpdates),
		updatesDone:            make(chan struct{}),
	}
}

func (a *Adapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (a *Adapter) PrepareCommandArgs() []string                   { return nil }

func (a *Adapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return protocol.ErrClosed
	}
	if a.client != nil {
		return errors.New("codex app-server adapter is already connected")
	}
	a.client = protocol.NewClient(stdin, stdout, protocol.Options{})
	a.client.SetNotificationHandler(a.handleNotification)
	a.client.SetRequestHandler(a.handleServerRequest)
	return nil
}

func (a *Adapter) Initialize(ctx context.Context) error {
	client := a.getClient()
	if client == nil {
		return errors.New("codex app-server adapter is not connected")
	}
	title := "Kandev"
	var response protocol.InitializeResponse
	if err := client.Call(ctx, protocol.MethodInitialize, protocol.InitializeParams{
		ClientInfo:   protocol.ClientInfo{Name: "kandev", Title: &title, Version: "1.0.0"},
		Capabilities: protocol.InitializeCapabilities{ExperimentalAPI: true},
	}, &response); err != nil {
		return fmt.Errorf("initialize Codex app-server: %w", err)
	}
	if err := client.Notify(ctx, protocol.MethodInitialized, struct{}{}); err != nil {
		return fmt.Errorf("complete Codex app-server initialization: %w", err)
	}
	models, err := a.fetchModels(ctx)
	if err != nil {
		return fmt.Errorf("read Codex model catalogue: %w", err)
	}
	name := strings.TrimSpace(response.UserAgent)
	if name == "" {
		name = "OpenAI Codex app-server"
	}
	a.mu.Lock()
	a.info = &AgentInfo{Name: name, Version: serverVersion}
	a.models = models
	a.mu.Unlock()
	return nil
}

func (a *Adapter) GetAgentInfo() *AgentInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.info == nil {
		return nil
	}
	copy := *a.info
	return &copy
}

func (a *Adapter) NewSession(ctx context.Context, servers []agenttypes.McpServer) (string, error) {
	client := a.getClient()
	if client == nil {
		return "", errors.New("codex app-server adapter is not connected")
	}
	policy := any("on-request")
	if a.cfg.AutoApprove {
		policy = "never"
	}
	params := protocol.ThreadStartParams{
		CWD:            a.cfg.WorkDir,
		ApprovalPolicy: policy,
		Sandbox:        "workspace-write",
		Config:         codexConfig(servers),
	}
	var response protocol.ThreadStartResponse
	if err := client.Call(ctx, protocol.MethodThreadStart, params, &response); err != nil {
		return "", fmt.Errorf("start Codex thread: %w", err)
	}
	if strings.TrimSpace(response.Thread.ID) == "" {
		return "", errors.New("start Codex thread: response omitted thread ID")
	}
	a.mu.Lock()
	a.threadID = response.Thread.ID
	a.turnID = ""
	a.promptPending = false
	a.children = make(map[string]childBinding)
	a.childStatuses = make(map[string]string)
	a.earlyChildActivities = make(map[string]string)
	a.backgrounds = make(map[string]protocol.BackgroundTerminal)
	a.backgroundSnapshotLoaded = false
	if response.Model != "" {
		a.modelID = response.Model
	}
	a.mu.Unlock()
	a.emit(streams.AgentEvent{Type: streams.EventTypeSessionStatus, SessionID: response.Thread.ID, SessionStatus: streams.SessionStatusNew})
	a.emitModelState()
	return response.Thread.ID, nil
}

func (a *Adapter) LoadSession(ctx context.Context, threadID string, servers []agenttypes.McpServer) error {
	client := a.getClient()
	if client == nil {
		return errors.New("codex app-server adapter is not connected")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return errors.New("resume Codex thread: thread ID is required")
	}
	policy := any("on-request")
	if a.cfg.AutoApprove {
		policy = "never"
	}
	params := protocol.ThreadResumeParams{
		ThreadID:       threadID,
		CWD:            a.cfg.WorkDir,
		ApprovalPolicy: policy,
		Sandbox:        "workspace-write",
		Config:         codexConfig(servers),
	}
	var response protocol.ThreadStartResponse
	if err := client.Call(ctx, protocol.MethodThreadResume, params, &response); err != nil {
		return fmt.Errorf("resume Codex thread %q: %w", threadID, err)
	}
	if response.Thread.ID != "" && response.Thread.ID != threadID {
		return errors.New("resume Codex thread: provider returned a different thread ID")
	}
	a.mu.Lock()
	a.threadID = threadID
	a.turnID = ""
	a.promptPending = false
	if response.Model != "" {
		a.modelID = response.Model
	}
	a.mu.Unlock()
	a.restoreThreadBindings(ctx, threadID)
	a.refreshBackgroundTerminals(ctx, threadID)
	a.emit(streams.AgentEvent{Type: streams.EventTypeSessionStatus, SessionID: threadID, SessionStatus: streams.SessionStatusResumed})
	a.emitModelState()
	return nil
}

// ForkSession creates a native child thread through a completed turn. It
// leaves this adapter bound to the source thread.
func (a *Adapter) ForkSession(ctx context.Context, sourceSessionID, completedTurnID string) (string, error) {
	a.mu.RLock()
	threadID, activeTurn := a.threadID, a.turnID
	activeChild := hasActiveChild(a.childStatuses, a.earlyChildActivities)
	activeBackground := len(a.backgrounds) != 0
	a.mu.RUnlock()
	if sourceSessionID == "" || sourceSessionID != threadID {
		return "", fmt.Errorf("%w: source does not match the active thread", protocol.ErrForkPrecondition)
	}
	if completedTurnID == "" {
		return "", fmt.Errorf("%w: a completed turn ID is required", protocol.ErrForkPrecondition)
	}
	if activeTurn != "" || activeChild || activeBackground {
		return "", fmt.Errorf("%w: thread must be idle without active background work", protocol.ErrForkPrecondition)
	}
	client := a.getClient()
	if client == nil {
		return "", fmt.Errorf("%w: app-server adapter is not connected", protocol.ErrForkPrecondition)
	}
	thread, err := client.ForkThread(ctx, protocol.ThreadForkParams{ThreadID: threadID, LastTurnID: &completedTurnID})
	if err != nil {
		return "", fmt.Errorf("fork Codex thread: %w", err)
	}
	return thread.ID, nil
}

func (a *Adapter) Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment, generation uint64) error {
	if len(attachments) != 0 {
		return errors.New("codex app-server image and file attachments are not supported yet")
	}
	client := a.getClient()
	if client == nil {
		return errors.New("codex app-server adapter is not connected")
	}
	a.mu.Lock()
	threadID := a.threadID
	modelID := a.modelID
	if threadID == "" {
		a.mu.Unlock()
		return errNoActiveThread
	}
	if a.turnID != "" || a.promptPending {
		a.mu.Unlock()
		return errors.New("codex app-server thread already has an active turn")
	}
	if generation == 0 {
		a.turnSequence++
		generation = a.turnSequence
	}
	a.activeGeneration = generation
	a.promptPending = true
	a.mu.Unlock()
	input := []protocol.UserInput{{Type: "text", Text: message}}
	params := protocol.TurnStartParams{ThreadID: threadID, Input: input}
	if modelID != "" {
		params.Model = &modelID
	}
	go func() {
		var response protocol.TurnStartResponse
		if err := client.Call(ctx, protocol.MethodTurnStart, params, &response); err != nil {
			if flushErr := client.FlushInbound(context.Background()); flushErr != nil {
				a.log.Warn("failed to flush Codex events before turn error", zap.Error(flushErr))
			}
			a.emitTerminal(threadID, "", generation, err.Error())
			return
		}
		if isTerminal(response.Turn.Status) {
			if err := client.FlushInbound(context.Background()); err != nil {
				a.log.Warn("failed to flush Codex events before turn completion", zap.Error(err))
			}
			a.emitTerminal(threadID, response.Turn.ID, generation, turnError(response.Turn))
		}
	}()
	return nil
}

func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	threadID, turnID := a.threadID, a.turnID
	a.mu.RUnlock()
	if threadID == "" || turnID == "" {
		return errors.New("codex app-server has no active turn to interrupt")
	}
	client := a.getClient()
	if client == nil {
		return errors.New("codex app-server adapter is not connected")
	}
	return client.Call(ctx, protocol.MethodTurnInterrupt, protocol.TurnInterruptParams{ThreadID: threadID, TurnID: turnID}, nil)
}

func (a *Adapter) Updates() <-chan streams.AgentEvent { return a.updates }

func (a *Adapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.threadID
}

func (a *Adapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.turnID
}

func (a *Adapter) SetPermissionHandler(handler agenttypes.PermissionHandler) {
	a.mu.Lock()
	a.permission = handler
	a.mu.Unlock()
}

func (a *Adapter) SetUserInputRequestHandler(handler agenttypes.UserInputRequestHandler) {
	a.mu.Lock()
	a.userInputRequest = handler
	a.mu.Unlock()
}

func (a *Adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	client := a.client
	backgroundCancel := a.backgroundCancel
	a.mu.Unlock()
	if backgroundCancel != nil {
		backgroundCancel()
	}
	var err error
	if client != nil {
		err = client.Close()
	}
	a.updatesOnce.Do(func() {
		close(a.updatesDone)
		a.updatesMu.Lock()
		close(a.updates)
		a.updatesMu.Unlock()
	})
	return err
}

func (a *Adapter) RequiresProcessKill() bool { return a.cfg.RequiresProcessKill }

func (a *Adapter) SetModel(_ context.Context, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return errors.New("codex model ID is required")
	}
	a.mu.Lock()
	a.modelID = modelID
	a.mu.Unlock()
	return nil
}

func (a *Adapter) GetSessionModelState() *streams.SessionModelState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return &streams.SessionModelState{
		CurrentModelID:       a.modelID,
		Models:               append([]streams.SessionModelInfo(nil), a.models...),
		ConfigOptionsSettled: true,
	}
}

func (a *Adapter) getClient() *protocol.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

func (a *Adapter) fetchModels(ctx context.Context) ([]streams.SessionModelInfo, error) {
	client := a.getClient()
	var response protocol.ModelListResponse
	if err := client.Call(ctx, protocol.MethodModelList, protocol.ModelListParams{}, &response); err != nil {
		return nil, err
	}
	models := make([]streams.SessionModelInfo, 0, len(response.Data))
	for _, model := range response.Data {
		id := model.Model
		if id == "" {
			id = model.ID
		}
		if strings.TrimSpace(id) == "" {
			continue
		}
		name := model.DisplayName
		if name == "" {
			name = id
		}
		models = append(models, streams.SessionModelInfo{ModelID: id, Name: name, Description: model.Description})
	}
	return models, nil
}

func (a *Adapter) emitModelState() {
	state := a.GetSessionModelState()
	a.emit(streams.AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      a.GetSessionID(),
		CurrentModelID: state.CurrentModelID,
		SessionModels:  state.Models,
	})
}

func (a *Adapter) handleNotification(_ context.Context, method string, raw json.RawMessage) {
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return
	}
	threadID := stringField(params, "threadId")
	activeThreadID := a.GetSessionID()
	turnID := notificationTurnID(params)
	itemID := stringField(params, "itemId")
	if threadID == "" {
		threadID = activeThreadID
	}
	isRootThread := threadID == activeThreadID
	rootThreadID, parentToolCallID, _ := a.eventScope(threadID)
	if turnID != "" && isRootThread {
		a.mu.Lock()
		a.turnID = turnID
		a.promptPending = false
		a.mu.Unlock()
	}
	switch method {
	case "turn/started":
		a.handleTurnStarted(threadID, rootThreadID, turnID, isRootThread)
	case "item/agentMessage/delta":
		a.emitTextDelta(params, rootThreadID, turnID, itemID, parentToolCallID, false)
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		a.emitTextDelta(params, rootThreadID, turnID, itemID, parentToolCallID, true)
	case "item/started":
		a.emitItem(params, rootThreadID, threadID, parentToolCallID, turnID, false)
	case "item/completed":
		a.emitItem(params, rootThreadID, threadID, parentToolCallID, turnID, true)
	case "turn/completed":
		a.handleTurnCompleted(params, threadID, rootThreadID, turnID, isRootThread)
	case protocol.NotificationTokenUsage:
		a.handleTokenUsageNotification(params)
	case protocol.NotificationRawResponse:
		a.handleRawResponseCompleted(params)
	}
}

func (a *Adapter) handleTurnStarted(threadID, rootThreadID, turnID string, isRootThread bool) {
	a.beginProviderTurn(threadID, turnID)
	if !isRootThread {
		a.emitChildStatus(threadID, childStatusRunning)
		return
	}
	a.mu.RLock()
	generation := a.activeGeneration
	a.mu.RUnlock()
	a.emit(streams.AgentEvent{Type: streams.EventTypeTurnStarted, SessionID: rootThreadID, OperationID: turnID, PromptGeneration: generation})
}

func (a *Adapter) emitTextDelta(params map[string]any, rootThreadID, turnID, itemID, parentToolCallID string, reasoning bool) {
	text := stringField(params, "delta")
	if text == "" {
		return
	}
	event := streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, SessionID: rootThreadID, OperationID: turnID,
		ProtocolMessageID: itemID, ParentToolCallID: parentToolCallID, Role: "assistant", Text: text,
	}
	if reasoning {
		event.Type = streams.EventTypeReasoning
		event.ReasoningSummary = text
		event.Text = ""
	}
	a.emit(event)
}

func (a *Adapter) handleTurnCompleted(params map[string]any, threadID, rootThreadID, turnID string, isRootThread bool) {
	turn := decodedCompletedTurn(params, turnID)
	a.finalizeProviderTurn(threadID, turn.ID)
	if !isRootThread {
		a.emitChildStatus(threadID, turn.Status)
		return
	}
	a.emitTerminal(rootThreadID, turn.ID, 0, turnError(turn))
	go a.refreshBackgroundTerminals(context.Background(), rootThreadID)
}

func decodedCompletedTurn(params map[string]any, turnID string) protocol.Turn {
	var turn protocol.Turn
	if rawTurn, ok := params["turn"]; ok {
		if data, err := json.Marshal(rawTurn); err == nil {
			_ = json.Unmarshal(data, &turn)
		}
	}
	if turn.ID == "" {
		turn.ID = turnID
	}
	return turn
}

func (a *Adapter) emitItem(params map[string]any, rootThreadID, sourceThreadID, parentToolCallID, turnID string, completed bool) {
	item, ok := params["item"].(map[string]any)
	if !ok {
		return
	}
	itemType := stringField(item, "type")
	switch itemType {
	case "collabAgentToolCall":
		a.emitCollabToolCall(item, rootThreadID, turnID, completed)
		return
	case "subAgentActivity":
		a.emitSubagentActivity(item)
		return
	case "commandExecution":
		a.emitCommandExecution(item, rootThreadID, parentToolCallID, completed)
		return
	}
	if itemType == "" || itemType == "agentMessage" || itemType == "reasoning" || itemType == "userMessage" || itemType == "plan" {
		return
	}
	itemID := stringField(item, "id")
	name := itemType
	if rawName, ok := item["toolName"].(string); ok && rawName != "" {
		name = rawName
	}
	title := stringField(item, "title")
	status := "started"
	typeName := streams.EventTypeToolCall
	if completed {
		status = "completed"
		typeName = streams.EventTypeToolUpdate
	}
	event := streams.AgentEvent{Type: typeName, SessionID: rootThreadID, OperationID: turnID, ParentToolCallID: parentToolCallID, ToolCallID: itemID, ToolName: name, ToolTitle: title, ToolStatus: status}
	if itemID == "" {
		event.ToolCallID = turnID + ":" + name
	}
	a.emit(event)
	_ = sourceThreadID
}

func (a *Adapter) emitTerminal(threadID, turnID string, generation uint64, message string) {
	a.mu.Lock()
	if generation == 0 {
		generation = a.activeGeneration
	}
	key := turnID
	if key == "" {
		key = fmt.Sprintf("%s:%d", threadID, generation)
	}
	if _, exists := a.completed[key]; exists {
		a.mu.Unlock()
		return
	}
	a.completed[key] = struct{}{}
	if a.turnID == turnID {
		a.turnID = ""
	}
	if a.activeGeneration == generation {
		a.activeGeneration = 0
		a.promptPending = false
	}
	a.mu.Unlock()
	if message != "" {
		a.emit(streams.AgentEvent{Type: streams.EventTypeError, SessionID: threadID, OperationID: turnID, PromptGeneration: generation, Error: message})
	}
	a.emit(streams.AgentEvent{Type: streams.EventTypeComplete, SessionID: threadID, OperationID: turnID, PromptGeneration: generation})
}

func (a *Adapter) emit(event streams.AgentEvent) {
	a.updatesMu.RLock()
	defer a.updatesMu.RUnlock()
	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()
	if closed || a.updatesDone == nil {
		return
	}
	select {
	case a.updates <- event:
	case <-a.updatesDone:
	}
}

func codexConfig(servers []agenttypes.McpServer) map[string]any {
	if len(servers) == 0 {
		return nil
	}
	mcpServers := make(map[string]any, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		entry := make(map[string]any)
		switch strings.ToLower(server.Type) {
		case "http", "sse", "streamable_http":
			entry["url"] = server.URL
			if len(server.Headers) != 0 {
				entry["http_headers"] = server.Headers
			}
		default:
			entry["command"] = server.Command
			entry["args"] = server.Args
			if len(server.Env) != 0 {
				entry["env"] = server.Env
			}
		}
		mcpServers[name] = entry
	}
	if len(mcpServers) == 0 {
		return nil
	}
	return map[string]any{"mcp_servers": mcpServers}
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func notificationTurnID(params map[string]any) string {
	if turnID := stringField(params, "turnId"); turnID != "" {
		return turnID
	}
	turn, _ := params["turn"].(map[string]any)
	return stringField(turn, "id")
}

func isTerminal(status string) bool {
	switch strings.ToLower(status) {
	case "completed", "failed", "interrupted", "cancelled":
		return true
	default:
		return false
	}
}

func turnError(turn protocol.Turn) string {
	if strings.EqualFold(turn.Status, "failed") && len(turn.Error) != 0 {
		var detail struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(turn.Error, &detail) == nil && detail.Message != "" {
			return detail.Message
		}
		return string(turn.Error)
	}
	return ""
}
