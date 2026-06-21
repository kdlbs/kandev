// Package main implements antigravity-acp, a thin ACP (Agent Client Protocol)
// shim for Google's Antigravity CLI (`agy`). The `agy` CLI does not speak ACP —
// its surfaces are an interactive TUI and a non-interactive `--print` mode — so
// Kandev would otherwise only integrate it as a raw passthrough terminal. This
// shim speaks ACP over stdin/stdout toward agentctl and drives `agy --print`
// per prompt underneath, so Antigravity renders as a structured chat dialog
// (like Codex) instead of a console.
//
// Scope: text turns. `agy` performs file reads/edits/commands internally and
// prints its final answer; the shim streams that answer as an agent message.
// Intermediate tool-call cards are not surfaced (agy exposes no structured
// stream), but on-disk changes still appear in Kandev's Changes panel.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
)

// logOutput is the writer for diagnostic logs (stderr; stdout is reserved for
// the ACP JSON-RPC stream). Tests may override it.
var logOutput io.Writer = os.Stderr

// sessionState tracks per-session data the shim needs across prompts.
type sessionState struct {
	cwd     string
	model   string
	hadTurn bool // true once a prompt completed, so the next uses --continue
}

// antigravityAgent implements acp.Agent by translating ACP calls into `agy`
// subprocess invocations.
type antigravityAgent struct {
	conn         *acp.AgentSideConnection
	defaultModel string

	mu       sync.Mutex
	sessions map[acp.SessionId]*sessionState
	cancels  map[acp.SessionId]context.CancelFunc
	nextID   int
}

func main() {
	ag := &antigravityAgent{
		defaultModel: parseModelFlag(os.Args),
		sessions:     map[acp.SessionId]*sessionState{},
		cancels:      map[acp.SessionId]context.CancelFunc{},
	}
	asc := acp.NewAgentSideConnection(ag, os.Stdout, os.Stdin)
	ag.conn = asc
	<-asc.Done()
}

// parseModelFlag extracts the --model value from argv, empty when unset.
func parseModelFlag(args []string) string {
	for i := 1; i < len(args); i++ {
		if args[i] == "--model" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(args[i], "--model="); ok {
			return v
		}
	}
	return ""
}

// Initialize advertises the shim's capabilities. LoadSession/ResumeSession are
// supported so Kandev can reconnect a session and continue the agy conversation
// via --continue.
func (a *antigravityAgent) Initialize(_ context.Context, _ acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: true,
		},
	}, nil
}

// NewSession allocates a session, records its working directory, pre-trusts the
// workspace so agy's first-run prompt never blocks the non-interactive run, and
// advertises the available models as a select config option.
func (a *antigravityAgent) NewSession(ctx context.Context, req acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	trustWorkspace(req.Cwd)

	a.mu.Lock()
	a.nextID++
	sid := acp.SessionId(fmt.Sprintf("antigravity-%d-%d", os.Getpid(), a.nextID))
	a.sessions[sid] = &sessionState{cwd: req.Cwd, model: a.defaultModel}
	a.mu.Unlock()

	return acp.NewSessionResponse{
		SessionId:     sid,
		ConfigOptions: a.modelConfigOptions(ctx, a.defaultModel),
	}, nil
}

// modelConfigOptions runs `agy models` and renders the result as a single
// "model" select option. Returns nil when listing fails so the host falls back
// to a free-text model field rather than an empty dropdown.
func (a *antigravityAgent) modelConfigOptions(ctx context.Context, current string) []acp.SessionConfigOption {
	out, err := exec.CommandContext(ctx, agyBinary, "models").Output()
	if err != nil {
		return nil
	}
	models := parseModels(string(out))
	if len(models) == 0 {
		return nil
	}
	opts := make(acp.SessionConfigSelectOptionsUngrouped, 0, len(models))
	for _, m := range models {
		opts = append(opts, acp.SessionConfigSelectOption{Value: acp.SessionConfigValueId(m.ID), Name: m.Name})
	}
	if current == "" {
		current = models[0].ID
	}
	modelCat := acp.SessionConfigOptionCategoryModel
	return []acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{
			Category:     &modelCat,
			CurrentValue: acp.SessionConfigValueId(current),
			Id:           "model",
			Name:         "Model",
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &opts},
			Type:         "select",
		},
	}}
}

// LoadSession re-registers a session for resume. agy itself threads history via
// --continue, so the shim only needs to remember the cwd and that a turn has
// happened.
func (a *antigravityAgent) LoadSession(_ context.Context, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	a.registerResumedSession(req.SessionId, req.Cwd)
	return acp.LoadSessionResponse{}, nil
}

// ResumeSession mirrors LoadSession for clients that resume without replaying
// history.
func (a *antigravityAgent) ResumeSession(_ context.Context, req acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	a.registerResumedSession(req.SessionId, req.Cwd)
	return acp.ResumeSessionResponse{}, nil
}

func (a *antigravityAgent) registerResumedSession(sid acp.SessionId, cwd string) {
	trustWorkspace(cwd)
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, ok := a.sessions[sid]; ok {
		st.cwd = cwd
		st.hadTurn = true
		return
	}
	a.sessions[sid] = &sessionState{cwd: cwd, model: a.defaultModel, hadTurn: true}
}

// Prompt runs one `agy --print` invocation for the session and streams its
// stdout back as an agent message. The first prompt in a session starts a fresh
// conversation; subsequent prompts add --continue so agy threads them.
func (a *antigravityAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	st := a.session(req.SessionId)
	if st == nil {
		return acp.PromptResponse{}, fmt.Errorf("unknown session %q", req.SessionId)
	}
	prompt := extractPromptText(req.Prompt)
	if strings.TrimSpace(prompt) == "" {
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	a.setCancel(req.SessionId, cancel)
	defer a.clearCancel(req.SessionId)

	args := buildPromptArgs(prompt, st.model, st.hadTurn)
	cmd := exec.CommandContext(runCtx, agyBinary, args...)
	cmd.Dir = st.cwd

	stopReason, err := a.runAndStream(runCtx, req.SessionId, cmd)
	if err != nil {
		return acp.PromptResponse{}, err
	}
	a.markTurnComplete(req.SessionId)
	return acp.PromptResponse{StopReason: stopReason}, nil
}

// runAndStream executes cmd, forwarding stdout line by line as agent message
// chunks. It returns Cancelled when the run context was cancelled, EndTurn on
// clean completion. stderr is captured for the error message on failure.
func (a *antigravityAgent) runAndStream(ctx context.Context, sid acp.SessionId, cmd *exec.Cmd) (acp.StopReason, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start agy: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var emitted bool
	for scanner.Scan() {
		emitted = true
		a.emitText(ctx, sid, scanner.Text()+"\n")
	}

	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return acp.StopReasonCancelled, nil
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return "", fmt.Errorf("agy failed: %s", msg)
	}
	if !emitted {
		a.emitText(ctx, sid, "(no output)")
	}
	return acp.StopReasonEndTurn, nil
}

func (a *antigravityAgent) emitText(ctx context.Context, sid acp.SessionId, text string) {
	if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sid,
		Update:    acp.UpdateAgentMessageText(text),
	}); err != nil {
		fmt.Fprintf(logOutput, "antigravity-acp: session update failed: %v\n", err)
	}
}

// Cancel aborts the in-flight prompt for a session, if any.
func (a *antigravityAgent) Cancel(_ context.Context, params acp.CancelNotification) error {
	a.mu.Lock()
	cancel := a.cancels[params.SessionId]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// SetSessionConfigOption updates the session model when the "model" option is
// set, so a mid-session model change takes effect on the next prompt.
func (a *antigravityAgent) SetSessionConfigOption(ctx context.Context, params acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	v := params.ValueId
	if v == nil || v.ConfigId != "model" {
		return acp.SetSessionConfigOptionResponse{}, nil
	}
	st := a.session(v.SessionId)
	if st == nil {
		return acp.SetSessionConfigOptionResponse{}, nil
	}
	a.mu.Lock()
	st.model = string(v.Value)
	model := st.model
	a.mu.Unlock()
	return acp.SetSessionConfigOptionResponse{ConfigOptions: a.modelConfigOptions(ctx, model)}, nil
}

// --- Unused ACP surface (no-ops) -------------------------------------------

func (a *antigravityAgent) Authenticate(_ context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *antigravityAgent) Logout(_ context.Context, _ acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}

func (a *antigravityAgent) CloseSession(_ context.Context, req acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	a.mu.Lock()
	delete(a.sessions, req.SessionId)
	delete(a.cancels, req.SessionId)
	a.mu.Unlock()
	return acp.CloseSessionResponse{}, nil
}

func (a *antigravityAgent) ListSessions(_ context.Context, _ acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (a *antigravityAgent) SetSessionMode(_ context.Context, _ acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

// --- helpers ----------------------------------------------------------------

func (a *antigravityAgent) session(sid acp.SessionId) *sessionState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[sid]
}

func (a *antigravityAgent) markTurnComplete(sid acp.SessionId) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, ok := a.sessions[sid]; ok {
		st.hadTurn = true
	}
}

func (a *antigravityAgent) setCancel(sid acp.SessionId, cancel context.CancelFunc) {
	a.mu.Lock()
	a.cancels[sid] = cancel
	a.mu.Unlock()
}

func (a *antigravityAgent) clearCancel(sid acp.SessionId) {
	a.mu.Lock()
	delete(a.cancels, sid)
	a.mu.Unlock()
}

// extractPromptText concatenates the text content blocks of an ACP prompt.
func extractPromptText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != nil {
			parts = append(parts, b.Text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
