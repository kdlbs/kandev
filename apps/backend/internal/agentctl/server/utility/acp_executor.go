package utility

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
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
	response, err := e.executeACPSession(ctx, stdin, stdout, workDir, req.Prompt, req.Mode)
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

// executeACPSession performs the ACP handshake, creates a session, optionally
// sets the session mode, sends the prompt, and collects the response text.
func (e *ACPInferenceExecutor) executeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
	prompt string,
	mode string,
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

	// Optionally set the session mode before prompting.
	if mode != "" {
		if _, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
			SessionId: sessionID,
			ModeId:    acp.SessionModeId(mode),
		}); err != nil {
			return "", fmt.Errorf("ACP session/set_mode failed: %w", err)
		}
	}

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

// Probe runs an ephemeral ACP handshake (initialize + session/new) to discover
// agent capabilities, auth methods, models, and modes. It does not send a prompt.
func (e *ACPInferenceExecutor) Probe(ctx context.Context, req *ProbeRequest) (*ProbeResponse, error) {
	if req.InferenceConfig == nil {
		return &ProbeResponse{Success: false, Error: "inference config is required"}, nil
	}
	cfg := req.InferenceConfig
	if len(cfg.Command) == 0 {
		return &ProbeResponse{Success: false, Error: "inference command is empty"}, nil
	}
	workDir := cfg.WorkDir
	if workDir == "" {
		return &ProbeResponse{Success: false, Error: "work_dir is required for ACP probe"}, nil
	}
	if err := validateCommandName(cfg.Command[0]); err != nil {
		return &ProbeResponse{Success: false, Error: err.Error()}, nil
	}

	startTime := time.Now()

	// Probes intentionally omit the model flag so session/new returns the agent's
	// default model and the complete availableModels list.
	args := buildACPCommand(cfg, "")

	e.logger.Info("starting ACP probe",
		zap.String("agent_id", req.AgentID),
		zap.Strings("command", args))

	//nolint:gosec // args[0] is validated above; args come from the trusted agent registry via InferenceConfig
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &ProbeResponse{Success: false, Error: fmt.Sprintf("stdin pipe: %v", err)}, nil
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &ProbeResponse{Success: false, Error: fmt.Sprintf("stdout pipe: %v", err)}, nil
	}
	if err := cmd.Start(); err != nil {
		return &ProbeResponse{Success: false, Error: fmt.Sprintf("start: %v", err)}, nil
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	resp, err := e.probeACPSession(ctx, stdin, stdout, workDir)
	if err != nil {
		return &ProbeResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	resp.Success = true
	resp.DurationMs = int(time.Since(startTime).Milliseconds())
	return resp, nil
}

// probeACPSession performs initialize + session/new and returns the parsed
// capabilities, without sending any prompt or running session/prompt.
func (e *ACPInferenceExecutor) probeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
) (*ProbeResponse, error) {
	client := acpclient.NewClient(
		acpclient.WithLogger(e.logger),
		acpclient.WithWorkspaceRoot(workDir),
	)

	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(slog.Default().With("component", "acp-probe"))

	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    "kandev-probe",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ACP initialize failed: %w", err)
	}

	sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return nil, fmt.Errorf("ACP session/new failed: %w", err)
	}

	out := buildInitProbeFields(initResp)
	applySessionProbeFields(out, sessionResp)
	return out, nil
}

// buildInitProbeFields populates agent info, protocol version, capabilities and
// auth methods from an ACP initialize response.
func buildInitProbeFields(initResp acp.InitializeResponse) *ProbeResponse {
	out := &ProbeResponse{
		ProtocolVersion: int(initResp.ProtocolVersion),
		LoadSession:     initResp.AgentCapabilities.LoadSession,
		PromptCapabilities: ProbePromptCapabilities{
			Image:           initResp.AgentCapabilities.PromptCapabilities.Image,
			Audio:           initResp.AgentCapabilities.PromptCapabilities.Audio,
			EmbeddedContext: initResp.AgentCapabilities.PromptCapabilities.EmbeddedContext,
		},
	}
	if initResp.AgentInfo != nil {
		out.AgentName = initResp.AgentInfo.Name
		out.AgentVersion = initResp.AgentInfo.Version
	}
	for _, m := range initResp.AuthMethods {
		out.AuthMethods = append(out.AuthMethods, ProbeAuthMethod{
			ID:          string(m.Id), //nolint:unconvert // AuthMethodId is a named string type; conversion required
			Name:        m.Name,
			Description: derefString(m.Description),
		})
	}
	return out
}

// applySessionProbeFields populates models and modes from an ACP session/new response.
func applySessionProbeFields(out *ProbeResponse, sessionResp acp.NewSessionResponse) {
	if sessionResp.Models != nil {
		out.CurrentModelID = string(sessionResp.Models.CurrentModelId)
		for _, m := range sessionResp.Models.AvailableModels {
			out.Models = append(out.Models, ProbeModel{
				ID:          string(m.ModelId),
				Name:        m.Name,
				Description: derefString(m.Description),
			})
		}
	}
	if sessionResp.Modes != nil {
		out.CurrentModeID = string(sessionResp.Modes.CurrentModeId)
		for _, m := range sessionResp.Modes.AvailableModes {
			out.Modes = append(out.Modes, ProbeMode{
				ID:          string(m.Id),
				Name:        m.Name,
				Description: derefString(m.Description),
			})
		}
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// allowedProbeCommands is the fixed set of executables a probe is permitted
// to launch. The agent registry always uses one of these to wrap an ACP CLI,
// so validating against this list satisfies CodeQL's taint analysis without
// restricting real usage.
var allowedProbeCommands = map[string]struct{}{
	"npx":      {},
	"auggie":   {},
	"opencode": {},
}

// validateCommandName returns an error if the command is not in the allow-list.
// This runs on the agentctl-server side before spawning the ACP subprocess.
func validateCommandName(name string) error {
	base := filepath.Base(name)
	if _, ok := allowedProbeCommands[base]; !ok {
		return fmt.Errorf("command %q is not an allowed ACP probe command", base)
	}
	return nil
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
