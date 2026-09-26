package utility

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
	"go.uber.org/zap"
)

const (
	codexNPXExecutable        = "npx"
	codexAppServerSubcommand  = "app-server"
	codexNpxYesFlag           = "--yes"
	codexNpmPreferOfflineFlag = "--prefer-offline"
	codexNpmPrefixFlag        = "--prefix"
)

var codexPackageSpec = regexp.MustCompile(`^@openai/codex@[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// CodexAppServerInferenceExecutor runs short utility prompts through the same
// native protocol as task sessions. It accepts only the built-in Codex package
// command because this endpoint crosses the agentctl process boundary.
type CodexAppServerInferenceExecutor struct {
	logger *zap.Logger
}

func NewCodexAppServerInferenceExecutor(log *zap.Logger) *CodexAppServerInferenceExecutor {
	if log == nil {
		log = zap.NewNop()
	}
	return &CodexAppServerInferenceExecutor{logger: log}
}

func (e *CodexAppServerInferenceExecutor) Probe(ctx context.Context, req *ProbeRequest) (*ProbeResponse, error) {
	if req == nil || req.InferenceConfig == nil {
		return &ProbeResponse{Error: "inference config is required"}, nil
	}
	if strings.TrimSpace(req.InferenceConfig.WorkDir) == "" {
		return &ProbeResponse{Error: "work_dir is required for Codex app-server probe"}, nil
	}
	command, args, err := resolveCodexAppServerCommand(req.InferenceConfig)
	if err != nil {
		return &ProbeResponse{Error: err.Error()}, nil
	}
	start := time.Now()
	client, cleanup, stderr, err := e.start(ctx, command, args, req.InferenceConfig)
	if err != nil {
		return &ProbeResponse{Error: fmt.Sprintf("start Codex app-server: %v", err), DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	defer cleanup()
	if err := initializeCodexAppServer(ctx, client); err != nil {
		return &ProbeResponse{Error: utilityUpstreamError(err, stderr.tail()), DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	var listed protocol.ModelListResponse
	if err := client.Call(ctx, protocol.MethodModelList, protocol.ModelListParams{}, &listed); err != nil {
		return &ProbeResponse{Error: utilityUpstreamError(err, stderr.tail()), DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	models := make([]ProbeModel, 0, len(listed.Data))
	for _, model := range listed.Data {
		id := model.Model
		if id == "" {
			id = model.ID
		}
		if id == "" {
			continue
		}
		name := model.DisplayName
		if name == "" {
			name = id
		}
		models = append(models, ProbeModel{ID: id, Name: name, Description: model.Description})
	}
	return &ProbeResponse{Success: true, AgentName: "OpenAI Codex app-server", AgentVersion: protocol.SupportedCodexVersion, Models: models, DurationMs: int(time.Since(start).Milliseconds())}, nil
}

func (e *CodexAppServerInferenceExecutor) Execute(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
	if req == nil || req.InferenceConfig == nil {
		return &PromptResponse{Error: "inference config is required"}, nil
	}
	if strings.TrimSpace(req.InferenceConfig.WorkDir) == "" {
		return &PromptResponse{Error: "work_dir is required for Codex app-server inference"}, nil
	}
	command, args, err := resolveCodexAppServerCommand(req.InferenceConfig)
	if err != nil {
		return &PromptResponse{Error: err.Error()}, nil
	}
	start := time.Now()
	client, cleanup, stderr, err := e.start(ctx, command, args, req.InferenceConfig)
	if err != nil {
		return &PromptResponse{Error: fmt.Sprintf("start Codex app-server: %v", err), DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	defer cleanup()
	if err := initializeCodexAppServer(ctx, client); err != nil {
		return &PromptResponse{Error: utilityUpstreamError(err, stderr.tail()), DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	model := strings.TrimSpace(req.Model)
	var modelPtr *string
	if model != "" {
		modelPtr = &model
	}
	ephemeral := true
	var thread protocol.ThreadStartResponse
	if err := client.Call(ctx, protocol.MethodThreadStart, protocol.ThreadStartParams{
		Model: modelPtr, CWD: req.InferenceConfig.WorkDir, Ephemeral: &ephemeral,
		ApprovalPolicy: "never", Sandbox: "read-only", Config: codexUtilityMCPConfig(req.MCPServers),
	}, &thread); err != nil {
		return &PromptResponse{Error: utilityUpstreamError(err, stderr.tail()), DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	if strings.TrimSpace(thread.Thread.ID) == "" {
		return &PromptResponse{Error: "Codex app-server thread/start omitted thread ID", DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	if thread.Model != "" {
		model = thread.Model
	}
	result, err := executeCodexUtilityTurn(ctx, client, thread.Thread.ID, model, req.Prompt)
	if err != nil {
		return &PromptResponse{Error: utilityUpstreamError(err, stderr.tail()), Model: model, DurationMs: int(time.Since(start).Milliseconds())}, nil
	}
	return &PromptResponse{Success: true, Response: result, Model: model, DurationMs: int(time.Since(start).Milliseconds())}, nil
}

func (e *CodexAppServerInferenceExecutor) start(
	ctx context.Context,
	command string,
	args []string,
	cfg *InferenceConfigDTO,
) (*protocol.Client, func(), *stderrBuffer, error) {
	args = append([]string(nil), args...)
	if err := managedruntime.PrepareNPMProjectPrefix(args); err != nil {
		return nil, nil, nil, fmt.Errorf("prepare managed npm project prefix: %w", err)
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = sanitizeEnvForAgent(cfg)
	configureACPCommand(cmd, e.logger)
	stderr := &stderrBuffer{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, stderr, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, stderr, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, stderr, fmt.Errorf("start: %w", err)
	}
	lifecycle, lifecycleErr := installACPCommandLifecycle(cmd)
	if lifecycleErr != nil {
		e.logger.Warn("failed to install Codex app-server process lifecycle", zap.Error(lifecycleErr))
	}
	client := protocol.NewClient(stdin, stdout, protocol.Options{})
	cleanup := func() {
		_ = client.Close()
		cleanupACPCommand(ctx, cmd, lifecycle, e.logger)
	}
	return client, cleanup, stderr, nil
}

func resolveCodexAppServerCommand(cfg *InferenceConfigDTO) (string, []string, error) {
	if cfg == nil {
		return "", nil, errors.New("inference config is required")
	}
	if cfg.OperatorDefined || len(cfg.CommandPrefix) != 0 || len(cfg.CLIFlags) != 0 {
		return "", nil, errors.New("codex app-server utility requires the built-in unwrapped command")
	}
	args := cfg.Command
	if isDirectCodexAppServerCommand(args) {
		return args[0], append([]string(nil), args[1:]...), nil
	}
	if isLegacyManagedCodexAppServerCommand(args) {
		return args[0], append([]string(nil), args[1:]...), nil
	}
	if isManagedCodexAppServerCommand(args) {
		return args[0], append([]string(nil), args[1:]...), nil
	}
	return "", nil, errors.New("codex app-server utility command is not allow-listed")
}

func isDirectCodexAppServerCommand(args []string) bool {
	return len(args) == 2 && args[0] == "codex" && args[1] == codexAppServerSubcommand
}

func isLegacyManagedCodexAppServerCommand(args []string) bool {
	return len(args) == 5 && args[0] == codexNPXExecutable && args[1] == codexNpxYesFlag &&
		args[2] == codexNpmPreferOfflineFlag && codexPackageSpec.MatchString(args[3]) && args[4] == codexAppServerSubcommand
}

func isManagedCodexAppServerCommand(args []string) bool {
	return len(args) == 7 && args[0] == codexNPXExecutable && args[1] == codexNpxYesFlag &&
		args[2] == codexNpmPreferOfflineFlag && args[3] == codexNpmPrefixFlag &&
		args[4] == managedruntime.NPMProjectPrefix && codexPackageSpec.MatchString(args[5]) && args[6] == codexAppServerSubcommand
}

func initializeCodexAppServer(ctx context.Context, client *protocol.Client) error {
	title := "Kandev utility"
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
	return nil
}

func executeCodexUtilityTurn(ctx context.Context, client *protocol.Client, threadID, model, prompt string) (string, error) {
	var mu sync.Mutex
	var response strings.Builder
	completed := make(chan protocol.Turn, 1)
	var completeOnce sync.Once
	client.SetNotificationHandler(utilityTurnNotificationHandler(&mu, &response, completed, &completeOnce))
	if prompt == "" {
		return "", errors.New("prompt is required")
	}
	input := []protocol.UserInput{{Type: "text", Text: prompt}}
	params := protocol.TurnStartParams{ThreadID: threadID, Input: input}
	if model != "" {
		params.Model = &model
	}
	var startResponse protocol.TurnStartResponse
	startErr := make(chan error, 1)
	go func() { startErr <- client.Call(ctx, protocol.MethodTurnStart, params, &startResponse) }()
	return waitForCodexUtilityTurn(ctx, startErr, completed, &startResponse, &mu, &response)
}

func utilityTurnNotificationHandler(
	mu *sync.Mutex,
	response *strings.Builder,
	completed chan<- protocol.Turn,
	completeOnce *sync.Once,
) protocol.NotificationHandler {
	return func(_ context.Context, method string, raw json.RawMessage) {
		var params map[string]any
		if err := json.Unmarshal(raw, &params); err != nil {
			return
		}
		switch method {
		case "item/agentMessage/delta":
			if delta, ok := params["delta"].(string); ok {
				mu.Lock()
				response.WriteString(delta)
				mu.Unlock()
			}
		case "turn/completed":
			value, ok := params["turn"]
			if !ok {
				completeOnce.Do(func() { completed <- protocol.Turn{Status: "failed"} })
				return
			}
			encoded, err := json.Marshal(value)
			var turn protocol.Turn
			if err != nil || json.Unmarshal(encoded, &turn) != nil {
				turn.Status = "failed"
			}
			completeOnce.Do(func() { completed <- turn })
		}
	}
}

func waitForCodexUtilityTurn(
	ctx context.Context,
	startErr <-chan error,
	completed <-chan protocol.Turn,
	startResponse *protocol.TurnStartResponse,
	mu *sync.Mutex,
	response *strings.Builder,
) (string, error) {
	for {
		select {
		case err := <-startErr:
			if err != nil {
				return "", fmt.Errorf("start Codex turn: %w", err)
			}
			if terminal, result, err := completedTurnResult(startResponse.Turn, mu, response); terminal {
				return result, err
			}
			startErr = nil
		case turn := <-completed:
			_, result, err := completedTurnResult(turn, mu, response)
			return result, err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func completedTurnResult(turn protocol.Turn, mu *sync.Mutex, response *strings.Builder) (bool, string, error) {
	if turn.Status != "completed" {
		if turn.Status == "failed" || turn.Status == "interrupted" {
			return true, "", fmt.Errorf("codex turn ended with status %q", turn.Status)
		}
		return false, "", nil
	}
	mu.Lock()
	defer mu.Unlock()
	return true, response.String(), nil
}

func codexUtilityMCPConfig(servers []MCPServerDTO) map[string]any {
	if len(servers) == 0 {
		return nil
	}
	mcpServers := make(map[string]any, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		url := strings.TrimSpace(server.URL)
		if name == "" || url == "" || (server.Type != "http" && server.Type != "sse" && server.Type != "streamable_http") {
			continue
		}
		entry := map[string]any{"url": url}
		if len(server.HeaderKVs) != 0 {
			headers := make(map[string]string, len(server.HeaderKVs))
			for _, header := range server.HeaderKVs {
				if strings.TrimSpace(header.Name) != "" {
					headers[header.Name] = header.Value
				}
			}
			entry["http_headers"] = headers
		}
		mcpServers[name] = entry
	}
	if len(mcpServers) == 0 {
		return nil
	}
	return map[string]any{"mcp_servers": mcpServers}
}

func utilityUpstreamError(err error, stderr string) string {
	_ = stderr // raw process diagnostics remain in the agentctl log only.
	return err.Error()
}
