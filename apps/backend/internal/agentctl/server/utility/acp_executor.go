package utility

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const (
	openCodeCommand       = "opencode"
	openCodeACPSubcommand = "acp"

	acpCommandTerminateGrace = 250 * time.Millisecond
	acpCommandForceKillGrace = 500 * time.Millisecond
	acpCommandPollInterval   = 25 * time.Millisecond
	acpProbeConfigWait       = 1 * time.Second
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
	model, modelConfigOptions, _ := acpcompat.MigrateCursorModel(req.AgentID, req.Model, nil)
	resolvedCmd := resolveProbeCommand(cfg.Command[0])
	if resolvedCmd == "" {
		return &PromptResponse{Success: false, Error: fmt.Sprintf("command %q is not an allowed ACP command", cfg.Command[0])}, nil
	}

	startTime := time.Now()

	// Build command with model flag
	args := buildACPCommand(cfg, model)

	e.logger.Info("starting ACP inference",
		zap.String("agent_id", req.AgentID),
		zap.String("model", model),
		zap.Strings("command", args))

	// Use the hard-coded resolvedCmd (not args[0]) so CodeQL can see that
	// the executable name is not derived from tainted input.
	//nolint:gosec // resolvedCmd is from a hard-coded allow-list; args[1:] are CLI flags
	cmdArgs := args[1:]
	if len(cfg.CommandPrefix) > 0 {
		args = append(append([]string{}, cfg.CommandPrefix...), args...)
		resolvedCmd = resolveProbeCommand(args[0])
		if resolvedCmd == "" {
			return &PromptResponse{Success: false, Error: fmt.Sprintf("command prefix %q is not an allowed ACP command", args[0])}, nil
		}
		cmdArgs = args[1:]
	}
	cmdArgs = append(cmdArgs, cfg.CLIFlags...)
	// Use the hard-coded resolvedCmd (not args[0]) so CodeQL can see that
	// the executable name is not derived from tainted input.
	cmd := exec.CommandContext(ctx, resolvedCmd, cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = sanitizeEnvForAgent(req.InferenceConfig)
	configureACPCommand(cmd, e.logger)

	// Same reasoning as the probe: without this the child's own account of why
	// it died is discarded.
	var stderr stderrBuffer
	cmd.Stderr = &stderr

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
	lifecycle, err := installACPCommandLifecycle(cmd)
	if err != nil {
		e.logger.Warn("failed to install ACP command lifecycle; falling back to process-tree cleanup",
			zap.Error(err))
	}

	defer cleanupACPCommand(ctx, cmd, lifecycle, e.logger)

	// Execute ACP protocol
	mcpServers, dropped := toACPMcpServers(req.MCPServers)
	for _, name := range dropped {
		e.logger.Warn("ACP inference: dropping unsupported MCP server transport",
			zap.String("name", name))
	}
	response, err := e.executeACPSession(ctx, stdin, stdout, workDir, req.AgentID, req.Prompt, model, modelConfigOptions, req.Mode, req.AutoApprovePermissions, mcpServers)
	if err != nil {
		e.logger.Error("ACP inference failed",
			zap.String("agent_id", req.AgentID),
			zap.Error(err),
			zap.String("stderr", stderr.tail()))
		return &PromptResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	return &PromptResponse{
		Success:    true,
		Response:   response,
		Model:      model,
		DurationMs: int(time.Since(startTime).Milliseconds()),
	}, nil
}

// executeACPSession performs the ACP handshake, creates a session, optionally
// sets the session model and mode, sends the prompt, and collects the response
// text. mcpServers, when non-empty, are forwarded to session/new so the agent
// can call MCP tools mid-prompt; an empty slice preserves the legacy "pure
// inference" behaviour.
func (e *ACPInferenceExecutor) executeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
	agentID string,
	prompt string,
	model string,
	modelConfigOptions map[string]string,
	mode string,
	autoApprovePermissions *bool,
	mcpServers []acp.McpServer,
) (string, error) {
	// Collect response text from updates
	var responseText strings.Builder
	var mu sync.Mutex

	updateHandler := func(n acp.SessionNotification) {
		if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
			chunk := sanitizeInferenceChunk(n.Update.AgentMessageChunk.Content.Text.Text)
			if chunk == "" {
				return
			}
			mu.Lock()
			responseText.WriteString(chunk)
			mu.Unlock()
		}
	}

	// Create ACP client
	clientOptions := []acpclient.ClientOption{
		acpclient.WithLogger(e.logger),
		acpclient.WithWorkspaceRoot(workDir),
		acpclient.WithUpdateHandler(updateHandler),
	}
	if autoApprovePermissions != nil && !*autoApprovePermissions {
		clientOptions = append(clientOptions, acpclient.WithPermissionHandler(func(context.Context, *agentctltypes.PermissionRequest) (*agentctltypes.PermissionResponse, error) {
			return &agentctltypes.PermissionResponse{Cancelled: true}, nil
		}))
	}
	client := acpclient.NewClient(clientOptions...)

	// Create ACP connection
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(slog.Default().With("component", "acp-inference"))

	// Initialize ACP handshake
	// Same client capabilities the session adapter and the probe send. This
	// path applies a caller-supplied model below, and an agent that picks its
	// model-picker mode from the handshake advertises a different id set per
	// mode — so opting out here would reject the very model ids the rest of
	// the product hands out.
	_, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Meta: acpcompat.ClientCapabilityMeta(agentID, nil),
		},
		ClientInfo: &acp.Implementation{
			Name:    "kandev-inference",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return "", describeACPFailure(ctx, "initialize", err)
	}

	// Create new session. ACP requires McpServers to be a non-nil slice;
	// callers without tools pass nil and we substitute an empty array here.
	if mcpServers == nil {
		mcpServers = []acp.McpServer{}
	}
	sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        workDir,
		McpServers: mcpServers,
	})
	if err != nil {
		return "", describeACPFailure(ctx, "session/new", err)
	}

	sessionID := sessionResp.SessionId

	// Optionally set the session model before prompting. ACP-first agents
	// declare no CLI ModelFlag, so `--model` is not appended at spawn time.
	// Model selection may be a model-shaped config option (Codex/Cursor) or
	// the older unstable session/set_model method, depending on the agent.
	if model != "" {
		if _, err := applySessionModel(ctx, conn, sessionID, model, sessionResp.ConfigOptions); err != nil {
			return "", fmt.Errorf("ACP model selection failed: %w", err)
		}
	}
	if _, err := applySessionConfigOptions(ctx, conn, string(sessionID), modelConfigOptions); err != nil {
		return "", fmt.Errorf("ACP model config selection failed: %w", err)
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

func applySessionModel(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID acp.SessionId,
	model string,
	configOptions []acp.SessionConfigOption,
) (sessionmodel.Method, error) {
	method, _, err := applySessionModelWithConfigOptions(
		ctx, conn, sessionID, model, configOptions,
	)
	return method, err
}

func applySessionModelWithConfigOptions(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID acp.SessionId,
	model string,
	configOptions []acp.SessionConfigOption,
) (sessionmodel.Method, []acp.SessionConfigOption, error) {
	return sessionmodel.ApplySDKWithConfigOptions(ctx, conn, sessionmodel.Request{
		SessionID:     string(sessionID),
		ModelID:       model,
		ConfigOptions: sessionmodel.FromACP(configOptions),
	})
}

func applySessionConfigOptions(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID string,
	options map[string]string,
) ([]acp.SessionConfigOption, error) {
	configOptions, _, err := applySessionConfigOptionsWithBoundary(
		ctx, conn, sessionID, options, nil,
	)
	return configOptions, err
}

// applySessionConfigOptionsWithBoundary applies a batch of option mutations
// and returns the notification version captured immediately before the final
// mutation. A snapshot from an earlier mutation is not authoritative for the
// batch, so probe callers use that boundary when they wait for a notification.
func applySessionConfigOptionsWithBoundary(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID string,
	options map[string]string,
	state *acpProbeNotificationState,
) ([]acp.SessionConfigOption, int, error) {
	if len(options) == 0 {
		return nil, 0, nil
	}
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var configOptions []acp.SessionConfigOption
	for _, key := range keys {
		previousVersion := 0
		if state != nil {
			previousVersion = state.currentConfigUpdateVersion()
		}
		response, err := conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
			ValueId: &acp.SetSessionConfigOptionValueId{
				SessionId: acp.SessionId(sessionID),
				ConfigId:  acp.SessionConfigId(key),
				Value:     acp.SessionConfigValueId(options[key]),
			},
		})
		if err != nil {
			return nil, 0, fmt.Errorf("set %q: %w", key, err)
		}
		configOptions = response.ConfigOptions
		if key == keys[len(keys)-1] {
			return configOptions, previousVersion, nil
		}
	}
	return configOptions, 0, nil
}

func filterRequestedConfigOptions(
	requested map[string]string,
	available []acp.SessionConfigOption,
) map[string]string {
	if len(requested) == 0 || len(available) == 0 {
		return nil
	}
	availableValues := make(map[string]map[string]struct{}, len(available))
	for _, option := range available {
		if option.Select == nil {
			continue
		}
		values := make(map[string]struct{})
		for _, choice := range selectOptionsUngrouped(option.Select.Options) {
			values[string(choice.Value)] = struct{}{}
		}
		availableValues[string(option.Select.Id)] = values
	}
	filtered := make(map[string]string, len(requested))
	for id, value := range requested {
		values, ok := availableValues[id]
		if !ok {
			continue
		}
		// Some providers expose a select option without enumerating choices.
		// Keep the persisted value in that case. When choices are present,
		// discard values from a previous model that the new model does not
		// support.
		if len(values) == 0 {
			filtered[id] = value
			continue
		}
		if _, ok := values[value]; ok {
			filtered[id] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// toACPMcpServers converts the cross-process DTO list into the ACP SDK shape.
// Returns nil when there are no entries so callers can use the nil-as-empty
// convention. The second return value carries the names of any DTOs we
// couldn't convert (unsupported transport, e.g. stdio) so the caller can
// surface them in logs rather than having them silently disappear from the
// agent's tool surface.
func toACPMcpServers(in []MCPServerDTO) ([]acp.McpServer, []string) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]acp.McpServer, 0, len(in))
	var dropped []string
	for _, s := range in {
		switch strings.ToLower(s.Type) {
		case "http":
			out = append(out, acp.McpServer{Http: &acp.McpServerHttpInline{
				Name:    s.Name,
				Type:    "http",
				Url:     s.URL,
				Headers: toACPHeaders(s.HeaderKVs),
			}})
		case "sse":
			out = append(out, acp.McpServer{Sse: &acp.McpServerSseInline{
				Name:    s.Name,
				Type:    "sse",
				Url:     s.URL,
				Headers: toACPHeaders(s.HeaderKVs),
			}})
		default:
			// Unsupported transport (stdio, or anything else). We don't fail
			// the whole inference call on a single bad entry — the agent can
			// still run with the entries that did convert — but we surface
			// the name so misconfiguration is visible in logs rather than
			// silently leaving the agent without tools it expected to have.
			dropped = append(dropped, s.Name)
		}
	}
	return out, dropped
}

func toACPHeaders(in []HTTPHeaderDTO) []acp.HttpHeader {
	if len(in) == 0 {
		return []acp.HttpHeader{}
	}
	out := make([]acp.HttpHeader, 0, len(in))
	for _, h := range in {
		out = append(out, acp.HttpHeader{Name: h.Name, Value: h.Value})
	}
	return out
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
	isOpenCode := isOpenCodeACPCommand(cfg.Command)
	var refreshedOpenCodeModels []ProbeModel
	if isOpenCode && req.Refresh {
		refreshedOpenCodeModels = e.loadOpenCodeModels(ctx, resolvedCmd, workDir, true)
	}

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
	cmd.Env = sanitizeEnvForAgent(req.InferenceConfig)
	configureACPCommand(cmd, e.logger)

	// Keep the child's stderr. A probe that dies before answering otherwise
	// reports only "peer disconnected before response", and the reason the
	// process gave — an npx resolution failure, a missing runtime, a panic —
	// goes to the void, leaving the UI's "check agent logs" pointing at logs
	// that never carried it.
	var stderr stderrBuffer
	cmd.Stderr = &stderr

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
	lifecycle, err := installACPCommandLifecycle(cmd)
	if err != nil {
		e.logger.Warn("failed to install ACP command lifecycle; falling back to process-tree cleanup",
			zap.Error(err))
	}
	defer cleanupACPCommand(ctx, cmd, lifecycle, e.logger)

	resp, err := e.probeACPSessionWithContext(
		ctx, stdin, stdout, workDir, req.AgentID, req.Model, req.Mode, req.ConfigOptions,
	)
	if err != nil {
		// The tail goes to the log rather than into the response: handlers
		// deliberately keep subprocess diagnostics and tmp paths out of client
		// payloads. An empty tail is itself a result — the child produced no
		// diagnostics, rather than producing some we failed to record.
		e.logger.Error("ACP probe failed",
			zap.String("agent_id", req.AgentID),
			zap.Error(err),
			zap.String("stderr", stderr.tail()))
		return &ProbeResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}
	if len(resp.Models) == 0 && isOpenCode {
		if len(refreshedOpenCodeModels) == 0 {
			refreshedOpenCodeModels = e.loadOpenCodeModels(ctx, resolvedCmd, workDir, false)
		}
		resp.Models = refreshedOpenCodeModels
	}

	resp.Success = true
	resp.DurationMs = int(time.Since(startTime).Milliseconds())
	return resp, nil
}

// isOpenCodeACPCommand reports whether the configured ACP probe command is
// OpenCode's ACP transport.
func isOpenCodeACPCommand(command []string) bool {
	return len(command) >= 2 &&
		filepath.Base(command[0]) == openCodeCommand &&
		command[1] == openCodeACPSubcommand
}

// loadOpenCodeModels runs OpenCode's CLI model listing and logs failures
// without failing the broader ACP capability probe.
func (e *ACPInferenceExecutor) loadOpenCodeModels(
	ctx context.Context,
	resolvedCmd string,
	workDir string,
	refresh bool,
) []ProbeModel {
	models, err := probeOpenCodeModels(ctx, resolvedCmd, workDir, refresh)
	if err != nil {
		e.logger.Warn("ACP probe: failed to list opencode models",
			zap.String("command", resolvedCmd),
			zap.Error(err))
		return nil
	}
	if len(models) == 0 {
		e.logger.Warn("ACP probe: opencode models returned no valid model entries",
			zap.String("command", resolvedCmd))
	}
	return models
}

// probeOpenCodeModels lists OpenCode's models, optionally refreshing its cache.
func probeOpenCodeModels(
	ctx context.Context,
	resolvedCmd string,
	workDir string,
	refresh bool,
) ([]ProbeModel, error) {
	args := []string{"models"}
	if refresh {
		args = append(args, "--refresh")
	}
	//nolint:gosec // resolvedCmd is from the same hard-coded allow-list used to launch the ACP probe.
	cmd := exec.CommandContext(ctx, resolvedCmd, args...)
	cmd.Dir = workDir
	cmd.Env = environWithNoColor(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return nil, commandErrorWithStderr(err)
	}
	return parseOpenCodeModelsOutput(string(out)), nil
}

// environWithNoColor returns an environment that forces NO_COLOR=1, replacing
// any caller-provided value.
func environWithNoColor(environ []string) []string {
	env := make([]string, 0, len(environ)+1)
	for _, item := range environ {
		if !strings.HasPrefix(item, "NO_COLOR=") {
			env = append(env, item)
		}
	}
	return append(env, "NO_COLOR=1")
}

// commandErrorWithStderr preserves stderr from failed commands when Go exposes
// it through exec.ExitError.
func commandErrorWithStderr(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := strings.TrimSpace(string(exitErr.Stderr))
		if stderr != "" {
			return fmt.Errorf("%w: %s", err, stderr)
		}
	}
	return err
}

// parseOpenCodeModelsOutput converts newline-delimited OpenCode model IDs into
// deduplicated probe model entries.
func parseOpenCodeModelsOutput(output string) []ProbeModel {
	seen := make(map[string]struct{})
	var models []ProbeModel
	for _, line := range strings.Split(output, "\n") {
		id := strings.TrimSpace(line)
		if !isOpenCodeModelID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, ProbeModel{ID: id, Name: id})
	}
	return models
}

// isOpenCodeModelID accepts model-like OpenCode IDs and rejects decoration or
// progress lines from CLI output.
func isOpenCodeModelID(id string) bool {
	return strings.Contains(id, "/") && !strings.ContainsAny(id, " \t\r")
}

type acpProbeNotificationState struct {
	agentID             string
	mu                  sync.Mutex
	commands            []ProbeCommand
	configOptions       []acp.SessionConfigOption
	configUpdateVersion int
	gotCommands         chan struct{}
	gotConfigOptions    chan struct{}
}

func newACPProbeNotificationState(agentID string) *acpProbeNotificationState {
	return &acpProbeNotificationState{
		agentID:          agentID,
		gotCommands:      make(chan struct{}, 1),
		gotConfigOptions: make(chan struct{}, 1),
	}
}

func (s *acpProbeNotificationState) handle(n acp.SessionNotification) {
	if update := n.Update.ConfigOptionUpdate; update != nil {
		s.mu.Lock()
		s.configOptions = append([]acp.SessionConfigOption(nil), update.ConfigOptions...)
		s.configUpdateVersion++
		s.mu.Unlock()
		select {
		case s.gotConfigOptions <- struct{}{}:
		default:
		}
	}
	if update := n.Update.AvailableCommandsUpdate; update != nil {
		s.mu.Lock()
		s.commands = s.commands[:0]
		for _, command := range update.AvailableCommands {
			s.commands = append(s.commands, ProbeCommand{
				Name:        command.Name,
				Description: acpcompat.NormalizeCommandDescription(s.agentID, command.Description),
			})
		}
		s.mu.Unlock()
		select {
		case s.gotCommands <- struct{}{}:
		default:
		}
	}
}

func (s *acpProbeNotificationState) currentConfigUpdateVersion() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configUpdateVersion
}

func (s *acpProbeNotificationState) waitForConfigUpdate(
	ctx context.Context,
	previousVersion int,
) ([]acp.SessionConfigOption, bool, error) {
	timer := time.NewTimer(acpProbeConfigWait)
	defer timer.Stop()
	for {
		select {
		case <-s.gotConfigOptions:
			s.mu.Lock()
			if s.configUpdateVersion > previousVersion {
				updated := append([]acp.SessionConfigOption(nil), s.configOptions...)
				s.mu.Unlock()
				return updated, true, nil
			}
			s.mu.Unlock()
		case <-timer.C:
			return nil, false, nil
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func (s *acpProbeNotificationState) commandsSnapshot() []ProbeCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ProbeCommand(nil), s.commands...)
}

func waitForProbeCommands(ctx context.Context, state *acpProbeNotificationState) {
	select {
	case <-state.gotCommands:
	case <-time.After(1 * time.Second):
	case <-ctx.Done():
	}
}

type acpSessionModeSetter interface {
	SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error)
}

func applyProbeMode(
	ctx context.Context,
	conn acpSessionModeSetter,
	sessionID acp.SessionId,
	mode string,
	state *acpProbeNotificationState,
) ([]acp.SessionConfigOption, error) {
	previousVersion := state.currentConfigUpdateVersion()
	if _, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
		SessionId: sessionID,
		ModeId:    acp.SessionModeId(mode),
	}); err != nil {
		return nil, fmt.Errorf("ACP session/set_mode failed: %w", err)
	}
	updated, received, err := state.waitForConfigUpdate(ctx, previousVersion)
	if err != nil {
		return nil, err
	}
	if !received {
		return nil, nil
	}
	return updated, nil
}

func applyProbeModel(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID acp.SessionId,
	model string,
	configOptions []acp.SessionConfigOption,
	state *acpProbeNotificationState,
) ([]acp.SessionConfigOption, error) {
	previousVersion := state.currentConfigUpdateVersion()
	method, returnedConfigOptions, err := applySessionModelWithConfigOptions(
		ctx, conn, sessionID, model, configOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("ACP model selection failed: %w", err)
	}
	if method == sessionmodel.MethodNone {
		return nil, fmt.Errorf("ACP provider does not support model selection")
	}
	if returnedConfigOptions != nil {
		return returnedConfigOptions, nil
	}
	updated, received, err := state.waitForConfigUpdate(ctx, previousVersion)
	if err != nil {
		return nil, err
	}
	if !received {
		if method == sessionmodel.MethodSetModel {
			// The legacy session/set_model RPC applied the model, but the agent
			// surfaces no per-model config options and pushes no follow-up
			// config-update notification (e.g. auggie, which advertises a flat
			// model list and answers session/set_model with an empty result).
			// That is a valid empty resolution, not a failure: the caller keeps
			// the session-advertised options.
			return nil, nil
		}
		// A typed session/set_config_option that returns neither inline options
		// nor a config-update notification leaves us without an authoritative
		// snapshot for the newly selected model. Keeping the pre-switch
		// session/new snapshot would report the previous model's options as the
		// current configuration, so treat this as a failure.
		return nil, fmt.Errorf("ACP model selection returned no configuration options")
	}
	return updated, nil
}

func applyProbeConfigOptions(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID acp.SessionId,
	requested map[string]string,
	available []acp.SessionConfigOption,
	state *acpProbeNotificationState,
) ([]acp.SessionConfigOption, error) {
	requested = filterRequestedConfigOptions(requested, available)
	if len(requested) == 0 {
		return nil, nil
	}
	returnedConfigOptions, previousVersion, err := applySessionConfigOptionsWithBoundary(
		ctx, conn, string(sessionID), requested, state,
	)
	if err != nil {
		return nil, fmt.Errorf("ACP model config selection failed: %w", err)
	}
	if returnedConfigOptions != nil {
		return returnedConfigOptions, nil
	}
	updated, received, err := state.waitForConfigUpdate(ctx, previousVersion)
	if err != nil {
		return nil, err
	}
	if !received {
		return nil, fmt.Errorf("ACP model config selection returned no configuration options")
	}
	return updated, nil
}

// probeACPSessionWithContext performs a model-aware probe with optional mode
// and configuration selections. The returned options are always the latest
// complete snapshot observed from the provider.
func (e *ACPInferenceExecutor) probeACPSessionWithContext(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
	agentID string,
	model string,
	mode string,
	requestedConfigOptions map[string]string,
) (*ProbeResponse, error) {
	updates := newACPProbeNotificationState(agentID)

	client := acpclient.NewClient(
		acpclient.WithLogger(e.logger),
		acpclient.WithWorkspaceRoot(workDir),
		acpclient.WithUpdateHandler(updates.handle),
	)

	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(slog.Default().With("component", "acp-probe"))

	// Advertise the same model-picker capability the live session adapter sends.
	// cursor-agent picks its model picker mode from this handshake, so a probe
	// that opts out reports the exploded fast=true model rows while sessions run
	// on the bare ids — the agent-models surface would then offer a model list no
	// session uses, and a model id the UI cannot select. The probe intentionally
	// omits the live adapter's terminal_output base capability.
	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Meta: acpcompat.ClientCapabilityMeta(agentID, nil),
		},
		ClientInfo: &acp.Implementation{
			Name:    "kandev-probe",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return nil, describeACPFailure(ctx, "initialize", err)
	}

	sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return nil, describeACPFailure(ctx, "session/new", err)
	}

	// Mode can change the provider's available options. Apply it before the
	// model and use the resulting notification, when present, as the next
	// model-selection snapshot.
	if mode != "" {
		updated, err := applyProbeMode(ctx, conn, sessionResp.SessionId, mode, updates)
		if err != nil {
			return nil, err
		}
		if updated != nil {
			sessionResp.ConfigOptions = updated
		}
	}

	if model != "" {
		updated, err := applyProbeModel(
			ctx, conn, sessionResp.SessionId, model, sessionResp.ConfigOptions, updates,
		)
		if err != nil {
			return nil, err
		}
		if updated != nil {
			sessionResp.ConfigOptions = updated
		}
	}
	if updated, err := applyProbeConfigOptions(
		ctx,
		conn,
		sessionResp.SessionId,
		requestedConfigOptions,
		sessionResp.ConfigOptions,
		updates,
	); err != nil {
		return nil, err
	} else if updated != nil {
		sessionResp.ConfigOptions = updated
	}

	// Agents that don't advertise commands (or push them later) simply yield
	// an empty Commands slice.
	waitForProbeCommands(ctx, updates)

	out := buildInitProbeFields(initResp)
	applySessionProbeFields(out, sessionResp, agentID)
	out.Commands = updates.commandsSnapshot()
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
		id, name, desc, meta := acpclient.AuthMethodFields(m)
		if id == "" && name == "" {
			continue
		}
		out.AuthMethods = append(out.AuthMethods, ProbeAuthMethod{
			ID:          id,
			Name:        name,
			Description: derefString(desc),
			Meta:        meta,
		})
	}
	return out
}

// applySessionProbeFields populates models and modes from an ACP session/new response.
//
// As of acp-go-sdk v0.13.5 the legacy unstable `models` field on
// NewSessionResponse was removed upstream; model and mode selection are
// surfaced through the typed `configOptions[]` carrier with
// `category: model | mode`. The kdlbs fork restores read-only parsing of the
// top-level `models` field via acp.LegacyModels for agents that haven't
// migrated yet (auggie 0.29.x). The legacy `modes` field is still present on
// the SDK type and we keep populating from it for older agents.
func applySessionProbeFields(out *ProbeResponse, sessionResp acp.NewSessionResponse, agentID string) {
	out.ConfigOptions = probeConfigOptions(sessionResp.ConfigOptions)
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
	applyConfigOptionsAsModels(out, sessionResp.ConfigOptions)
	if len(out.Models) == 0 {
		applyLegacyModelsFallback(out, sessionResp.LegacyModels)
	}
	if sessionResp.Modes == nil {
		applyConfigOptionsAsModes(out, sessionResp.ConfigOptions)
	}
	applySessionCompatibility(out, agentID)
}

func applySessionCompatibility(out *ProbeResponse, agentID string) {
	models := make([]acpcompat.Model, 0, len(out.Models))
	for _, model := range out.Models {
		models = append(models, acpcompat.Model{ID: model.ID, Name: model.Name, Meta: model.Meta})
	}
	base := make([]streams.ConfigOption, 0, len(out.ConfigOptions))
	for _, option := range out.ConfigOptions {
		converted := streams.ConfigOption{
			Type:         option.Type,
			ID:           option.ID,
			Name:         option.Name,
			Description:  option.Description,
			CurrentValue: option.CurrentValue,
			Category:     option.Category,
		}
		for _, choice := range option.Options {
			converted.Options = append(converted.Options, streams.ConfigOptionValue{
				Value:       choice.Value,
				Name:        choice.Name,
				Description: choice.Description,
			})
		}
		base = append(base, converted)
	}
	normalized := acpcompat.NormalizeSessionConfig(agentID, base, models, out.CurrentModelID)
	out.ConfigOptions = make([]ProbeConfigOption, 0, len(normalized))
	for _, option := range normalized {
		converted := ProbeConfigOption{
			Type:         option.Type,
			ID:           option.ID,
			Name:         option.Name,
			Description:  option.Description,
			CurrentValue: option.CurrentValue,
			Category:     option.Category,
		}
		for _, choice := range option.Options {
			converted.Options = append(converted.Options, ProbeConfigOptionChoice{
				Value:       choice.Value,
				Name:        choice.Name,
				Description: choice.Description,
			})
		}
		out.ConfigOptions = append(out.ConfigOptions, converted)
	}
}

// applyLegacyModelsFallback fills out.Models / out.CurrentModelID from the
// pre-v0.13.5 top-level `models` field still emitted by agents like
// auggie 0.29.x. Only invoked when typed configOptions[category=model] did
// not produce a model list, so the new surface always wins when present.
func applyLegacyModelsFallback(out *ProbeResponse, legacy *acp.LegacyModels) {
	if legacy == nil || len(legacy.AvailableModels) == 0 {
		return
	}
	out.CurrentModelID = legacy.CurrentModelId
	for _, m := range legacy.AvailableModels {
		out.Models = append(out.Models, ProbeModel{
			ID:          m.ModelId,
			Name:        m.Name,
			Description: derefString(m.Description),
			Meta:        m.Meta,
		})
	}
}

func probeConfigOptions(opts []acp.SessionConfigOption) []ProbeConfigOption {
	out := make([]ProbeConfigOption, 0, len(opts))
	for _, opt := range opts {
		sel := opt.Select
		if sel == nil {
			continue
		}
		config := ProbeConfigOption{
			Type:         sel.Type,
			ID:           string(sel.Id),
			Name:         sel.Name,
			Description:  derefString(sel.Description),
			CurrentValue: string(sel.CurrentValue),
		}
		if sel.Category != nil {
			config.Category = string(*sel.Category)
		}
		for _, item := range selectOptionsUngrouped(sel.Options) {
			config.Options = append(config.Options, ProbeConfigOptionChoice{
				Value:       string(item.Value),
				Name:        item.Name,
				Description: derefString(item.Description),
			})
		}
		out = append(out, config)
	}
	return out
}

// applyConfigOptionsAsModels extracts a ProbeModel list from any
// configOptions[] entry tagged with category=model. Used as a fallback when
// the legacy `models` field is omitted by the agent.
func applyConfigOptionsAsModels(out *ProbeResponse, opts []acp.SessionConfigOption) {
	sel := findSelectConfigOption(opts, acp.SessionConfigOptionCategoryModel)
	if sel == nil {
		return
	}
	out.CurrentModelID = string(sel.CurrentValue)
	for _, opt := range selectOptionsUngrouped(sel.Options) {
		out.Models = append(out.Models, ProbeModel{
			ID:          string(opt.Value),
			Name:        opt.Name,
			Description: derefString(opt.Description),
			Meta:        opt.Meta,
		})
	}
}

// applyConfigOptionsAsModes mirrors applyConfigOptionsAsModels for modes.
func applyConfigOptionsAsModes(out *ProbeResponse, opts []acp.SessionConfigOption) {
	sel := findSelectConfigOption(opts, acp.SessionConfigOptionCategoryMode)
	if sel == nil {
		return
	}
	out.CurrentModeID = string(sel.CurrentValue)
	for _, opt := range selectOptionsUngrouped(sel.Options) {
		out.Modes = append(out.Modes, ProbeMode{
			ID:          string(opt.Value),
			Name:        opt.Name,
			Description: derefString(opt.Description),
			Meta:        opt.Meta,
		})
	}
}

// findSelectConfigOption returns the first Select-typed configOption whose
// category matches. Boolean toggles and other categories are skipped.
func findSelectConfigOption(opts []acp.SessionConfigOption, want acp.SessionConfigOptionCategory) *acp.SessionConfigOptionSelect {
	for i := range opts {
		sel := opts[i].Select
		if sel == nil || sel.Category == nil {
			continue
		}
		if *sel.Category == want {
			return sel
		}
	}
	return nil
}

// selectOptionsUngrouped flattens a SessionConfigSelectOptions union to a
// plain slice. Grouped options are flattened group-by-group so callers do not
// need to care about the nesting.
func selectOptionsUngrouped(opts acp.SessionConfigSelectOptions) []acp.SessionConfigSelectOption {
	if opts.Ungrouped != nil {
		return []acp.SessionConfigSelectOption(*opts.Ungrouped)
	}
	if opts.Grouped == nil {
		return nil
	}
	var out []acp.SessionConfigSelectOption
	for _, g := range *opts.Grouped {
		out = append(out, g.Options...)
	}
	return out
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
	"auggie":        "auggie",
	"cursor-agent":  "cursor-agent",
	"devin":         "devin",
	"grok":          "grok",
	"hermes":        "hermes",
	"kimi":          "kimi",
	"kiro-cli-chat": "kiro-cli-chat",
	"mock-agent":    "mock-agent",
	"npx":           "npx",
	"omp":           "omp",
	openCodeCommand: openCodeCommand,
	"qodercli":      "qodercli",
	"traecli":       "traecli",
}

// resolveProbeCommand validates and returns a hard-coded executable name for
// the given command. Returns the empty string if the command is not allowed.
//
// An agent launched from an absolute path arrives here carrying the executable
// suffix Windows requires — "mock-agent.exe" — which never matches the bare
// allow-list key, so its probe is refused before it can spawn.
//
// Only ".exe" is trimmed, and only on Windows. That is the suffix the Go build
// emits, so it is the only one an allow-listed binary can arrive with; trimming
// whatever filepath.Ext returns would instead let "mock-agent.cmd" or
// "mock-agent.txt" reach an allow-listed entry. Unix executables carry no such
// convention at all, and trimming there would let "opencode.sh" pass as
// "opencode".
func resolveProbeCommand(name string) string {
	base := filepath.Base(name)
	if resolved, ok := allowedProbeCommands[base]; ok {
		return resolved
	}
	if runtime.GOOS == "windows" {
		if ext := filepath.Ext(base); strings.EqualFold(ext, ".exe") {
			return allowedProbeCommands[strings.TrimSuffix(base, ext)]
		}
	}
	return ""
}

// stderrTailLimit bounds how much of a spawned agent's stderr reaches the log:
// enough for an npm failure or the head of a panic, small enough that a chatty
// agent cannot flood it.
const stderrTailLimit = 4 << 10

// stderrBuffer collects a spawned agent's stderr.
//
// os/exec copies stderr on a goroutine of its own whenever the writer is not an
// *os.File, and the tail is read while the child is still being torn down —
// cleanup calls Wait from a defer, after the response has been built. Writer and
// reader therefore overlap, and the mutex is what makes that legal: an
// unsynchronised bytes.Buffer here trips the race detector in
// TestProbeCleansUpDescendantProcessOnTimeout.
type stderrBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write keeps only the trailing stderrTailLimit bytes, so an agent that chatters
// for the length of a probe cannot grow this without bound. Trimming on read
// instead would still have retained every byte the child ever wrote. The full
// length of p is reported as written: a short count means failure to io.Writer,
// and discarding an old prefix is not a failure to record the new bytes.
func (b *stderrBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(p)
	if len(p) > stderrTailLimit {
		p = p[len(p)-stderrTailLimit:]
	}
	b.buf.Write(p)
	if excess := b.buf.Len() - stderrTailLimit; excess > 0 {
		b.buf.Next(excess)
	}
	return written, nil
}

// tail returns what was retained, trimmed — the end of the output rather than
// the beginning, because what a dying process printed last is what explains it.
// It is a best-effort snapshot: the child may still be writing while its process
// group is torn down, so the tail can be short or empty.
func (b *stderrBuffer) tail() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}

// describeACPFailure names an expired or cancelled context as the cause instead
// of reporting whatever the SDK saw on the wire.
//
// When the deadline fires, cleanup kills the child's process group; the ACP
// client then hits EOF on stdout and returns "peer disconnected before
// response". That makes a plain timeout indistinguishable from an agent that
// crashed — two failures calling for opposite responses, one of which is to
// wait longer and the other to go read the agent's own output.
func describeACPFailure(ctx context.Context, phase string, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("ACP %s timed out: %w", phase, ctx.Err())
	case errors.Is(ctx.Err(), context.Canceled):
		return fmt.Errorf("ACP %s cancelled: %w", phase, ctx.Err())
	default:
		return fmt.Errorf("ACP %s failed: %w", phase, err)
	}
}

// sanitizeEnvForAgent returns a child-process environment with agent-declared
// variables (InferenceConfigDTO.StripEnv) removed. Applied to one-shot
// probe/inference subprocesses; the persistent session path strips in
// process.Manager.buildAdapterConfig instead.
func sanitizeEnvForAgent(cfg *InferenceConfigDTO) []string {
	env := os.Environ()
	if cfg != nil {
		for key, value := range cfg.Env {
			env = RemoveEnvEntry(env, key)
			env = append(env, key+"="+value)
		}
		for _, key := range cfg.StripEnv {
			env = RemoveEnvEntry(env, key)
		}
	}
	return env
}

// RemoveEnvEntry removes all entries for the given key from the env slice.
// Used to ensure a variable is truly absent (not just empty) in the child
// process environment — some programs distinguish unset from empty string.
func RemoveEnvEntry(env []string, key string) []string {
	prefix := key + "="
	next := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			next = append(next, e)
		}
	}
	return next
}

// buildACPCommand builds the command arguments for ACP inference. The model
// parameter is a no-op for ACP-first agents (they have no ModelFlag); model
// selection is applied through the ACP session after session/new instead.
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

var piVersionBannerLineRE = regexp.MustCompile(`^\s*pi v\d+\.\d+\.\d+\s*$`)

// sanitizeInferenceChunk removes known non-content banner lines emitted by
// some CLIs (e.g. pi-acp printing "pi vX.Y.Z") so utility outputs like
// commit-message generation only contain model response content.
// Note: pi-acp is always launched via "npx" (see PiACP.InferenceConfig),
// so "npx" is the allowedProbeCommand entry that gates execution here.
func sanitizeInferenceChunk(chunk string) string {
	if chunk == "" {
		return ""
	}
	lines := strings.Split(chunk, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if piVersionBannerLineRE.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n")
}
