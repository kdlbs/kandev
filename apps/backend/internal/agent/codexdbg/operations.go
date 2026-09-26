package codexdbg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/codexappserver"
)

const maxModelPages = 50

// ProbeResult contains initialization capabilities and the model catalog read
// without creating a native thread or starting a turn.
type ProbeResult struct {
	Initialize codexappserver.InitializeResponse
	Models     []codexappserver.Model
	Features   []json.RawMessage
	Version    string
	SchemaHash string
}

// TurnResult describes one explicitly requested root turn.
type TurnResult struct {
	ThreadID           string
	TurnID             string
	Status             string
	CompletionObserved bool
	InterruptRequested bool
	NotificationsSeen  int
}

// MCPProbeResult separates app-server configuration from observed sentinel
// traffic.
type MCPProbeResult struct {
	ThreadID           string
	Configured         bool
	Connected          bool
	InitializeObserved bool
	ToolsListObserved  bool
	ToolCallObserved   bool
	ToolCount          int
	ToolCallSucceeded  bool
}

type appNotification struct {
	method string
	params json.RawMessage
}

// Initialize completes the app-server handshake and enables experimental
// events declared by the pinned protocol schema.
func Initialize(ctx context.Context, client *codexappserver.Client) (codexappserver.InitializeResponse, error) {
	var result codexappserver.InitializeResponse
	params := codexappserver.InitializeParams{
		ClientInfo: codexappserver.ClientInfo{
			Name:    "kandev-codexdbg",
			Version: "0.1.0",
		},
		Capabilities: codexappserver.InitializeCapabilities{
			ExperimentalAPI:    true,
			RequestAttestation: false,
		},
	}
	if err := client.Call(ctx, codexappserver.MethodInitialize, params, &result); err != nil {
		return result, fmt.Errorf("initialize app-server: %w", err)
	}
	if err := client.Notify(ctx, codexappserver.MethodInitialized, nil); err != nil {
		return result, fmt.Errorf("complete app-server initialization: %w", err)
	}
	return result, nil
}

// Probe reads initialization metadata and all model pages. It does not create
// a thread or start a model turn.
func Probe(ctx context.Context, client *codexappserver.Client) (ProbeResult, error) {
	initialize, err := Initialize(ctx, client)
	if err != nil {
		return ProbeResult{}, err
	}
	result := ProbeResult{
		Initialize: initialize,
		Version:    initialize.UserAgent,
		SchemaHash: codexappserver.SchemaSHA256V0154,
	}
	seenCursors := make(map[string]struct{})
	var cursor *string
	for page := 0; page < maxModelPages; page++ {
		params := codexappserver.ModelListParams{Cursor: cursor}
		var response codexappserver.ModelListResponse
		if err := client.Call(ctx, codexappserver.MethodModelList, params, &response); err != nil {
			return ProbeResult{}, fmt.Errorf("read app-server model catalog: %w", err)
		}
		result.Models = append(result.Models, response.Data...)
		if response.NextCursor == nil || *response.NextCursor == "" {
			break
		}
		if _, repeated := seenCursors[*response.NextCursor]; repeated {
			return ProbeResult{}, errors.New("app-server model catalog repeated a pagination cursor")
		}
		seenCursors[*response.NextCursor] = struct{}{}
		cursor = response.NextCursor
		if page == maxModelPages-1 {
			return ProbeResult{}, fmt.Errorf("app-server model catalog exceeds %d pages", maxModelPages)
		}
	}
	featureCursors := make(map[string]struct{})
	var featureCursor *string
	for page := 0; page < maxModelPages; page++ {
		params := struct {
			Cursor *string `json:"cursor,omitempty"`
			Limit  *int    `json:"limit,omitempty"`
		}{Cursor: featureCursor}
		var response codexappserver.ExperimentalFeatureListResponse
		if err := client.Call(ctx, codexappserver.MethodExperimentalFeatureList, params, &response); err != nil {
			return ProbeResult{}, fmt.Errorf("read app-server feature catalog: %w", err)
		}
		result.Features = append(result.Features, response.Data...)
		if response.NextCursor == nil || *response.NextCursor == "" {
			return result, nil
		}
		if _, repeated := featureCursors[*response.NextCursor]; repeated {
			return ProbeResult{}, errors.New("app-server feature catalog repeated a pagination cursor")
		}
		featureCursors[*response.NextCursor] = struct{}{}
		featureCursor = response.NextCursor
		if page == maxModelPages-1 {
			return ProbeResult{}, fmt.Errorf("app-server feature catalog exceeds %d pages", maxModelPages)
		}
	}
	return result, nil
}

// ReadThread returns provider history for an explicitly named thread.
func ReadThread(ctx context.Context, client *codexappserver.Client, threadID string) (codexappserver.Thread, error) {
	if strings.TrimSpace(threadID) == "" {
		return codexappserver.Thread{}, errors.New("thread id is required")
	}
	if _, err := Initialize(ctx, client); err != nil {
		return codexappserver.Thread{}, err
	}
	var result codexappserver.ThreadResponse
	if err := client.Call(ctx, codexappserver.MethodThreadRead, codexappserver.ThreadReadParams{ThreadID: threadID, IncludeTurns: true}, &result); err != nil {
		return codexappserver.Thread{}, fmt.Errorf("read thread %q: %w", threadID, err)
	}
	return result.Thread, nil
}

// ResumeThread resumes an explicitly named provider thread.
func ResumeThread(ctx context.Context, client *codexappserver.Client, threadID, workdir string) (codexappserver.Thread, error) {
	if strings.TrimSpace(threadID) == "" {
		return codexappserver.Thread{}, errors.New("thread id is required")
	}
	if _, err := Initialize(ctx, client); err != nil {
		return codexappserver.Thread{}, err
	}
	params := codexappserver.ThreadResumeParams{ThreadID: threadID, CWD: workdir}
	var result codexappserver.ThreadResponse
	if err := client.Call(ctx, codexappserver.MethodThreadResume, params, &result); err != nil {
		return codexappserver.Thread{}, fmt.Errorf("resume thread %q: %w", threadID, err)
	}
	return result.Thread, nil
}

// ForkThread forks an explicitly selected completed turn. It never retries an
// ambiguous RPC response.
func ForkThread(ctx context.Context, client *codexappserver.Client, threadID, throughTurnID string) (codexappserver.Thread, error) {
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(throughTurnID) == "" {
		return codexappserver.Thread{}, errors.New("thread id and through-turn id are required")
	}
	if _, err := Initialize(ctx, client); err != nil {
		return codexappserver.Thread{}, err
	}
	var result codexappserver.ThreadStartResponse
	params := codexappserver.ThreadForkParams{ThreadID: threadID, LastTurnID: &throughTurnID}
	if err := client.Call(ctx, codexappserver.MethodThreadFork, params, &result); err != nil {
		return codexappserver.Thread{}, fmt.Errorf("fork thread %q through turn %q: %w", threadID, throughTurnID, err)
	}
	return result.Thread, nil
}

// RunPrompt starts a provider thread, submits one prompt, and observes its root
// completion. linger bounds post-turn capture of child or background events.
func RunPrompt(ctx context.Context, client *codexappserver.Client, prompt, workdir string, linger time.Duration) (TurnResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return TurnResult{}, errors.New("prompt is required")
	}
	if _, err := Initialize(ctx, client); err != nil {
		return TurnResult{}, err
	}
	thread, err := startThread(ctx, client, workdir, false)
	if err != nil {
		return TurnResult{}, err
	}
	return runTurn(ctx, client, thread.ID, prompt, nil, linger)
}

// ResumePrompt sends one prompt to an explicitly resumed provider thread.
func ResumePrompt(ctx context.Context, client *codexappserver.Client, threadID, prompt, workdir string, linger time.Duration) (TurnResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return TurnResult{}, errors.New("prompt is required")
	}
	thread, err := ResumeThread(ctx, client, threadID, workdir)
	if err != nil {
		return TurnResult{}, err
	}
	return runTurn(ctx, client, thread.ID, prompt, nil, linger)
}

// InterruptPrompt sends an explicit prompt then requests interruption after a
// bounded delay. It reports root completion separately from the interrupt RPC.
func InterruptPrompt(ctx context.Context, client *codexappserver.Client, prompt, workdir string, after, linger time.Duration) (TurnResult, error) {
	if after < 0 {
		return TurnResult{}, errors.New("interrupt delay cannot be negative")
	}
	if strings.TrimSpace(prompt) == "" {
		return TurnResult{}, errors.New("prompt is required")
	}
	if _, err := Initialize(ctx, client); err != nil {
		return TurnResult{}, err
	}
	thread, err := startThread(ctx, client, workdir, false)
	if err != nil {
		return TurnResult{}, err
	}
	return runTurn(ctx, client, thread.ID, prompt, &after, linger)
}

func startThread(ctx context.Context, client *codexappserver.Client, workdir string, ephemeral bool) (codexappserver.Thread, error) {
	params := codexappserver.ThreadStartParams{CWD: workdir}
	if ephemeral {
		params.Ephemeral = &ephemeral
	}
	var result codexappserver.ThreadStartResponse
	if err := client.Call(ctx, codexappserver.MethodThreadStart, params, &result); err != nil {
		return codexappserver.Thread{}, fmt.Errorf("start app-server thread: %w", err)
	}
	if result.Thread.ID == "" {
		return codexappserver.Thread{}, errors.New("app-server returned an empty thread id")
	}
	return result.Thread, nil
}

func runTurn(ctx context.Context, client *codexappserver.Client, threadID, prompt string, interruptAfter *time.Duration, linger time.Duration) (TurnResult, error) {
	if linger < 0 {
		return TurnResult{}, errors.New("post-turn linger cannot be negative")
	}
	params := codexappserver.TurnStartParams{
		ThreadID: threadID,
		Input:    []codexappserver.UserInput{{Type: "text", Text: prompt, TextElements: []any{}}},
	}
	events, overflow, stopObserving := observeNotifications(client)
	defer stopObserving()
	var start codexappserver.TurnStartResponse
	if err := client.Call(ctx, codexappserver.MethodTurnStart, params, &start); err != nil {
		return TurnResult{}, fmt.Errorf("start app-server turn: %w", err)
	}
	if start.Turn.ID == "" {
		return TurnResult{}, errors.New("app-server returned an empty turn id")
	}
	return waitForTurn(ctx, client, threadID, start.Turn.ID, interruptAfter, linger, events, overflow)
}

func observeNotifications(client *codexappserver.Client) (<-chan appNotification, <-chan struct{}, func()) {
	events := make(chan appNotification, 512)
	overflow := make(chan struct{})
	var overflowOnce sync.Once
	client.SetNotificationHandler(func(_ context.Context, method string, params json.RawMessage) {
		select {
		case events <- appNotification{method: method, params: append(json.RawMessage(nil), params...)}:
		default:
			overflowOnce.Do(func() { close(overflow) })
		}
	})
	return events, overflow, func() { client.SetNotificationHandler(nil) }
}

func waitForTurn(ctx context.Context, client *codexappserver.Client, threadID, turnID string, interruptAfter *time.Duration, linger time.Duration, events <-chan appNotification, overflow <-chan struct{}) (TurnResult, error) {
	if (interruptAfter != nil && *interruptAfter < 0) || linger < 0 {
		return TurnResult{}, errors.New("turn observation durations cannot be negative")
	}

	result := TurnResult{ThreadID: threadID, TurnID: turnID}
	var interrupt *time.Timer
	var interruptChannel <-chan time.Time
	if interruptAfter != nil {
		interrupt = time.NewTimer(*interruptAfter)
		interruptChannel = interrupt.C
		defer interrupt.Stop()
	}
	result, err := awaitTurnCompletion(ctx, client, threadID, turnID, interruptChannel, events, overflow, result)
	if err != nil {
		return result, err
	}
	if linger == 0 {
		return result, nil
	}
	return observePostTurn(ctx, client, linger, events, overflow, result)
}

func awaitTurnCompletion(
	ctx context.Context,
	client *codexappserver.Client,
	threadID, turnID string,
	interruptChannel <-chan time.Time,
	events <-chan appNotification,
	overflow <-chan struct{},
	result TurnResult,
) (TurnResult, error) {
	for !result.CompletionObserved {
		select {
		case notification := <-events:
			result.NotificationsSeen++
			if notification.method == codexappserver.NotificationTurnComplete {
				var completion struct {
					ThreadID string              `json:"threadId"`
					Turn     codexappserver.Turn `json:"turn"`
				}
				if err := json.Unmarshal(notification.params, &completion); err == nil && completion.ThreadID == threadID && completion.Turn.ID == turnID {
					result.CompletionObserved = true
					result.Status = completion.Turn.Status
				}
			}
		case <-interruptChannel:
			interruptChannel = nil
			if err := client.Call(ctx, codexappserver.MethodTurnInterrupt, codexappserver.TurnInterruptParams{ThreadID: threadID, TurnID: turnID}, nil); err != nil {
				return result, fmt.Errorf("interrupt app-server turn: %w", err)
			}
			result.InterruptRequested = true
		case <-overflow:
			return result, errors.New("app-server notification queue exceeded 512 events")
		case <-ctx.Done():
			return result, ctx.Err()
		case <-client.Done():
			return result, fmt.Errorf("app-server disconnected before turn completion: %w", client.Err())
		}
	}
	return result, nil
}

func observePostTurn(ctx context.Context, client *codexappserver.Client, linger time.Duration, events <-chan appNotification, overflow <-chan struct{}, result TurnResult) (TurnResult, error) {
	timer := time.NewTimer(linger)
	defer timer.Stop()
	for {
		select {
		case <-events:
			result.NotificationsSeen++
		case <-overflow:
			return result, errors.New("app-server notification queue exceeded 512 events")
		case <-timer.C:
			return result, nil
		case <-ctx.Done():
			return result, ctx.Err()
		case <-client.Done():
			return result, fmt.Errorf("app-server disconnected during post-turn capture: %w", client.Err())
		}
	}
}

// MCPProbe uses direct app-server MCP status and tool calls. It does not ask a
// model to invoke the sentinel.
func MCPProbe(ctx context.Context, client *codexappserver.Client, sentinel *MCPSentinel, workdir string) (MCPProbeResult, error) {
	if sentinel == nil {
		return MCPProbeResult{}, errors.New("MCP sentinel is required")
	}
	if _, err := Initialize(ctx, client); err != nil {
		return MCPProbeResult{}, err
	}
	thread, err := startThread(ctx, client, workdir, true)
	if err != nil {
		return MCPProbeResult{}, err
	}
	result := MCPProbeResult{ThreadID: thread.ID}
	statusParams := codexappserver.MCPServerStatusListParams{ThreadID: &thread.ID, Detail: "full"}
	var status codexappserver.MCPServerStatusListResponse
	if err := client.Call(ctx, codexappserver.MethodMCPStatusList, statusParams, &status); err != nil {
		return result, fmt.Errorf("read MCP server status: %w", err)
	}
	for _, item := range status.Data {
		var server struct {
			Name          string          `json:"name"`
			RuntimeStatus json.RawMessage `json:"runtimeStatus"`
		}
		if json.Unmarshal(item, &server) == nil && server.Name == mcpSentinelName {
			result.Configured = true
			var runtime string
			if json.Unmarshal(server.RuntimeStatus, &runtime) == nil && runtime == "connected" {
				result.Connected = true
			}
		}
	}
	if !result.Configured {
		sentinelState := sentinel.Summary()
		result.InitializeObserved = sentinelState.InitializeObserved
		result.ToolsListObserved = sentinelState.ToolsListObserved
		result.ToolCallObserved = sentinelState.ToolCallObserved
		result.ToolCount = sentinelState.ToolCount
		return result, nil
	}
	call := codexappserver.MCPToolCallParams{
		ThreadID:  thread.ID,
		Server:    mcpSentinelName,
		Tool:      mcpSentinelTool,
		Arguments: json.RawMessage(`{}`),
	}
	var response struct {
		Content []json.RawMessage `json:"content"`
		IsError bool              `json:"isError"`
	}
	if err := client.Call(ctx, codexappserver.MethodMCPToolCall, call, &response); err != nil {
		return result, fmt.Errorf("call MCP sentinel: %w", err)
	}
	result.ToolCallSucceeded = !response.IsError
	sentinelState := sentinel.Summary()
	result.InitializeObserved = sentinelState.InitializeObserved
	result.ToolsListObserved = sentinelState.ToolsListObserved
	result.ToolCallObserved = sentinelState.ToolCallObserved
	result.ToolCount = sentinelState.ToolCount
	return result, nil
}
