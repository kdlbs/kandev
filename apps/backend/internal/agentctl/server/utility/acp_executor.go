package utility

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"go.uber.org/zap"
)

// ACPInferenceExecutor executes one-shot prompts using the ACP protocol.
// It spawns a new agent process, performs the ACP handshake, sends the prompt,
// collects the response, and tears down the process.
type ACPInferenceExecutor struct {
	logger *zap.Logger
}

// NewACPInferenceExecutor creates a new ACP inference executor.
func NewACPInferenceExecutor(logger *zap.Logger) *ACPInferenceExecutor {
	return &ACPInferenceExecutor{logger: logger}
}

// Execute runs a one-shot prompt using the ACP protocol.
func (e *ACPInferenceExecutor) Execute(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
	if req.InferenceConfig == nil {
		return &PromptResponse{Success: false, Error: "inference config is required"}, nil
	}

	cfg := req.InferenceConfig
	if len(cfg.Command) == 0 {
		return &PromptResponse{Success: false, Error: "inference command is empty"}, nil
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		return &PromptResponse{Success: false, Error: "work_dir is required for ACP inference"}, nil
	}

	startTime := time.Now()

	// Build command with model flag
	args := buildACPCommand(cfg, req.Model)

	e.logger.Info("starting ACP inference",
		zap.String("agent_id", req.AgentID),
		zap.String("model", req.Model),
		zap.Strings("command", args))

	// Start the agent process
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &PromptResponse{Success: false, Error: fmt.Sprintf("stdin pipe: %v", err)}, nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &PromptResponse{Success: false, Error: fmt.Sprintf("stdout pipe: %v", err)}, nil
	}

	if err := cmd.Start(); err != nil {
		return &PromptResponse{Success: false, Error: fmt.Sprintf("start: %v", err)}, nil
	}

	// Ensure process cleanup
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Execute ACP protocol
	response, err := e.executeACPSession(ctx, stdin, stdout, workDir, req.Prompt)
	if err != nil {
		return &PromptResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	return &PromptResponse{
		Success:    true,
		Response:   response,
		Model:      req.Model,
		DurationMs: int(time.Since(startTime).Milliseconds()),
	}, nil
}

// executeACPSession performs the ACP handshake, creates a session, sends the prompt,
// and collects the response text.
func (e *ACPInferenceExecutor) executeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
	prompt string,
) (string, error) {
	// Collect response text from updates
	var responseText strings.Builder
	var mu sync.Mutex

	updateHandler := func(n acp.SessionNotification) {
		if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
			mu.Lock()
			responseText.WriteString(n.Update.AgentMessageChunk.Content.Text.Text)
			mu.Unlock()
		}
	}

	// Create ACP client
	client := acpclient.NewClient(
		acpclient.WithLogger(e.logger),
		acpclient.WithWorkspaceRoot(workDir),
		acpclient.WithUpdateHandler(updateHandler),
	)

	// Create ACP connection
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(slog.Default().With("component", "acp-inference"))

	// Initialize ACP handshake
	_, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    "kandev-inference",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return "", fmt.Errorf("ACP initialize failed: %w", err)
	}

	// Create new session with empty MCP servers array (required by ACP protocol)
	sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return "", fmt.Errorf("ACP session/new failed: %w", err)
	}

	sessionID := sessionResp.SessionId

	// Send prompt and wait for completion
	_, err = conn.Prompt(ctx, acp.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		return "", fmt.Errorf("ACP prompt failed: %w", err)
	}

	mu.Lock()
	result := strings.TrimSpace(responseText.String())
	mu.Unlock()

	return result, nil
}

// buildACPCommand builds the command arguments for ACP inference.
func buildACPCommand(cfg *InferenceConfigDTO, model string) []string {
	args := make([]string, len(cfg.Command))
	copy(args, cfg.Command)

	if model != "" && len(cfg.ModelFlag) > 0 {
		for _, part := range cfg.ModelFlag {
			args = append(args, strings.ReplaceAll(part, "{model}", model))
		}
	}

	return args
}
