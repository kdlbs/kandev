package utility

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"go.uber.org/zap"
)

const (
	openCodeCommand       = "opencode"
	openCodeACPSubcommand = "acp"
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
	mcpServers, dropped := toACPMcpServers(req.MCPServers)
	for _, name := range dropped {
		e.logger.Warn("ACP inference: dropping unsupported MCP server transport",
			zap.String("name", name))
	}
	response, err := e.executeACPSession(ctx, stdin, stdout, workDir, req.Prompt, req.Model, req.Mode, mcpServers)
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
// text. mcpServers, when non-empty, are forwarded to session/new so the agent
// can call MCP tools mid-prompt; an empty slice preserves the legacy "pure
// inference" behaviour.
func (e *ACPInferenceExecutor) executeACPSession(
	ctx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	workDir string,
	prompt string,
	model string,
	mode string,
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
	if len(resp.Models) == 0 && isOpenCodeACPCommand(cfg.Command) {
		e.applyOpenCodeModelsFallback(ctx, resp, resolvedCmd, workDir)
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

// applyOpenCodeModelsFallback fills an otherwise empty probe model list from
// OpenCode's CLI model listing.
func (e *ACPInferenceExecutor) applyOpenCodeModelsFallback(ctx context.Context, resp *ProbeResponse, resolvedCmd, workDir string) {
	models, err := probeOpenCodeModels(ctx, resolvedCmd, workDir)
	if err != nil {
		e.logger.Warn("ACP probe: failed to list opencode models",
			zap.String("command", resolvedCmd),
			zap.Error(err))
		return
	}
	if len(models) == 0 {
		e.logger.Warn("ACP probe: opencode models returned no valid model entries",
			zap.String("command", resolvedCmd))
		return
	}
	resp.Models = models
}

// probeOpenCodeModels runs the OpenCode model-listing command and converts its
// output into probe model entries.
func probeOpenCodeModels(ctx context.Context, resolvedCmd, workDir string) ([]ProbeModel, error) {
	//nolint:gosec // resolvedCmd is from the same hard-coded allow-list used to launch the ACP probe.
	cmd := exec.CommandContext(ctx, resolvedCmd, "models")
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
// The legacy `models`/`modes` fields were marked UNSTABLE in the ACP spec and
// newer agents (e.g. claude-agent-acp v0.42+) drop them in favour of the
// general `configOptions[]` carrier with `category: model | mode`. Read the
// legacy fields first, then fall back to configOptions when they are absent
// so both shapes work.
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
	// Gate the fallback on the legacy field being absent (nil) rather than
	// producing an empty slice. A non-nil `Models` with an empty
	// `AvailableModels` is schema-valid and would otherwise mix sources:
	// `CurrentModelID` set from the legacy field, then immediately
	// overwritten by the configOptions fallback. Same for modes.
	if sessionResp.Models == nil {
		applyConfigOptionsAsModels(out, sessionResp.ConfigOptions)
	}
	if sessionResp.Modes == nil {
		applyConfigOptionsAsModes(out, sessionResp.ConfigOptions)
	}
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
