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
	resolvedCmd := resolveProbeCommand(cfg.Command[0])
	if resolvedCmd == "" {
		return &PromptResponse{Success: false, Error: fmt.Sprintf("command %q is not an allowed ACP command", cfg.Command[0])}, nil
	}

	startTime := time.Now()

	// Build command with model flag
	args := buildACPCommand(cfg, req.Model)

	e.logger.Info("starting ACP inference",
		zap.String("agent_id", req.AgentID),
		zap.String("model", req.Model),
		zap.Strings("command", args))

	// Use the hard-coded resolvedCmd (not args[0]) so CodeQL can see that
	// the executable name is not derived from tainted input.
	//nolint:gosec // resolvedCmd is from a hard-coded allow-list; args[1:] are CLI flags
	cmd := exec.CommandContext(ctx, resolvedCmd, args[1:]...)
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
	response, err := e.executeACPSession(ctx, stdin, stdout, workDir, req.Prompt, req.Model, req.Mode)
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
// sets the session model and mode, sends the prompt, and collects the response
// text.
func (e *ACPInferenceExecutor) executeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
	prompt string,
	model string,
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

	// Optionally set the session model before prompting. ACP-first agents
	// declare no CLI ModelFlag, so `--model` is not appended at spawn time;
	// the model has to be applied over the ACP protocol here.
	if model != "" {
		if _, err := conn.UnstableSetSessionModel(ctx, acp.UnstableSetSessionModelRequest{
			SessionId: sessionID,
			ModelId:   acp.UnstableModelId(model),
		}); err != nil {
			return "", fmt.Errorf("ACP session/set_model failed: %w", err)
		}
	}

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
	resolvedCmd := resolveProbeCommand(cfg.Command[0])
	if resolvedCmd == "" {
		return &ProbeResponse{Success: false, Error: fmt.Sprintf("command %q is not an allowed ACP probe command", cfg.Command[0])}, nil
	}

	startTime := time.Now()

	// Probes intentionally omit the model flag so session/new returns the agent's
	// default model and the complete availableModels list.
	args := buildACPCommand(cfg, "")

	e.logger.Info("starting ACP probe",
		zap.String("agent_id", req.AgentID),
		zap.Strings("command", args))

	// Use the hard-coded resolvedCmd (not args[0]) so CodeQL can see that
	// the executable name is not derived from tainted input.
	//nolint:gosec // resolvedCmd is from a hard-coded allow-list; args[1:] are CLI flags
	cmd := exec.CommandContext(ctx, resolvedCmd, args[1:]...)
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
// capabilities, without sending any prompt or running session/prompt. After
// session/new, it briefly drains out-of-band notifications to capture the
// `available_commands_update` notification which some agents emit post-session.
func (e *ACPInferenceExecutor) probeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
) (*ProbeResponse, error) {
	var mu sync.Mutex
	var commands []ProbeCommand
	gotCommands := make(chan struct{}, 1)
	updateHandler := func(n acp.SessionNotification) {
		if n.Update.AvailableCommandsUpdate == nil {
			return
		}
		mu.Lock()
		commands = commands[:0]
		for _, c := range n.Update.AvailableCommandsUpdate.AvailableCommands {
			commands = append(commands, ProbeCommand{Name: c.Name, Description: c.Description})
		}
		mu.Unlock()
		select {
		case gotCommands <- struct{}{}:
		default:
		}
	}

	client := acpclient.NewClient(
		acpclient.WithLogger(e.logger),
		acpclient.WithWorkspaceRoot(workDir),
		acpclient.WithUpdateHandler(updateHandler),
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

	// Wait up to 1s for the available_commands_update notification. Agents
	// that don't advertise commands (or push them later) simply yield an
	// empty Commands slice.
	select {
	case <-gotCommands:
	case <-time.After(1 * time.Second):
	case <-ctx.Done():
	}

	out := buildInitProbeFields(initResp)
	applySessionProbeFields(out, sessionResp)
	mu.Lock()
	out.Commands = append([]ProbeCommand(nil), commands...)
	mu.Unlock()
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
			Meta:        m.Meta,
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
				Meta:        m.Meta,
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
				Meta:        m.Meta,
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

// allowedProbeCommands maps each permitted executable base name to a
// constant string literal. Spawning must pass one of these literal strings
// to exec.Command so CodeQL's taint tracker can see that the command name
// is not derived from untrusted input — even though the value is
// semantically the same as the base name taken from InferenceConfig.Command.
var allowedProbeCommands = map[string]string{
	"npx":      "npx",
	"auggie":   "auggie",
	"opencode": "opencode",
}

// resolveProbeCommand validates and returns a hard-coded executable name for
// the given command. Returns the empty string if the command is not allowed.
func resolveProbeCommand(name string) string {
	return allowedProbeCommands[filepath.Base(name)]
}

// buildACPCommand builds the command arguments for ACP inference. The model
// parameter is a no-op for ACP-first agents (they have no ModelFlag); model
// selection is applied via the ACP session/set_model protocol call instead.
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
