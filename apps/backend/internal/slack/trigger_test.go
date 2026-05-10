package slack

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	"github.com/kandev/kandev/internal/utility/template"
)

// --- prefix / parser tests ---

func TestStartsWithPrefix(t *testing.T) {
	cases := []struct {
		text   string
		prefix string
		want   bool
	}{
		{"!kandev fix login bug", "!kandev", true},
		{"  !kandev fix login bug", "!kandev", true},
		{"!kandev: do the thing", "!kandev", true},
		{"!kandev", "!kandev", true},
		{"!kandevops handle this", "!kandev", false},
		{"hey !kandev fix this", "!kandev", false},
		{"> !kandev quoted reply", "!kandev", true},
		{"!KANDEV case insensitive", "!kandev", true},
		{"", "!kandev", false},
	}
	for _, c := range cases {
		if got := startsWithPrefix(c.text, c.prefix); got != c.want {
			t.Errorf("startsWithPrefix(%q, %q) = %v, want %v", c.text, c.prefix, got, c.want)
		}
	}
}

func TestStripPrefix(t *testing.T) {
	cases := []struct {
		text, prefix, want string
	}{
		{"!kandev fix login bug", "!kandev", "fix login bug"},
		{"  !kandev   fix login bug  ", "!kandev", "fix login bug"},
		{"!kandev: handle this", "!kandev", "handle this"},
		{"!kandev,please", "!kandev", "please"},
		{"> !kandev quoted reply", "!kandev", "quoted reply"},
		{"!kandev", "!kandev", ""},
	}
	for _, c := range cases {
		if got := stripPrefix(c.text, c.prefix); got != c.want {
			t.Errorf("stripPrefix(%q, %q) = %q, want %q", c.text, c.prefix, got, c.want)
		}
	}
}

func TestNewMatchesAfter(t *testing.T) {
	matches := []SlackMessage{
		{TS: "100.000005", Text: "!kandev five"},
		{TS: "100.000003", Text: "!kandev three"},
		{TS: "100.000004", Text: "!kandevops noop"},
		{TS: "100.000002", Text: "!kandev two"},
	}
	got := newMatchesAfter(matches, "100.000003", "!kandev")
	if len(got) != 1 || got[0].TS != "100.000005" {
		t.Errorf("expected only the freshest matching message, got %+v", got)
	}
}

func TestNewMatchesAfter_OrdersOldestFirst(t *testing.T) {
	matches := []SlackMessage{
		{TS: "100.000010", Text: "!kandev ten"},
		{TS: "100.000005", Text: "!kandev five"},
		{TS: "100.000007", Text: "!kandev seven"},
	}
	got := newMatchesAfter(matches, "", "!kandev")
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].TS != "100.000005" || got[1].TS != "100.000007" || got[2].TS != "100.000010" {
		t.Errorf("expected oldest-first order, got %v", []string{got[0].TS, got[1].TS, got[2].TS})
	}
}

func TestCompareTS(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "100.0", -1},
		{"100.0", "", 1},
		{"100.0", "100.0", 0},
		{"100.0", "200.0", -1},
		{"200.0", "100.0", 1},
	}
	for _, c := range cases {
		if got := compareTS(c.a, c.b); got != c.want {
			t.Errorf("compareTS(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// --- prompt composition ---

func TestComposePrompt_DefaultSystemWhenEmptyTemplate(t *testing.T) {
	r := &Runner{}
	thread := []SlackMessage{{TS: "100", UserName: "alice", Text: "thread message"}}
	prompt := r.composePrompt("", "",
		SlackMessage{UserName: "alice"}, "do thing", "https://slack/p100", thread)
	if !strings.Contains(prompt, "Kandev triage assistant") {
		t.Error("expected default system prompt when template is empty")
	}
	if !strings.Contains(prompt, "https://slack/p100") {
		t.Error("expected permalink in appended block")
	}
	if !strings.Contains(prompt, "do thing") {
		t.Error("expected instruction in appended block")
	}
}

func TestComposePrompt_TemplateOwnsLayoutWhenSubstituted(t *testing.T) {
	r := &Runner{}
	rawTemplate := "Custom: {{SlackInstruction}} for {{KandevWorkspaceID}}"
	resolved := "Custom: do thing for ws-1"
	prompt := r.composePrompt(rawTemplate, resolved, SlackMessage{}, "do thing", "", nil)
	if prompt != "Custom: do thing for ws-1" {
		t.Errorf("expected the resolved template only, got: %q", prompt)
	}
}

func TestComposePrompt_FallsBackWhenTemplateHasNoVars(t *testing.T) {
	// Template with no Slack-specific vars → resolved == raw (after trim) →
	// fall back so the agent still gets the Slack context block appended.
	r := &Runner{}
	template := "Be concise."
	prompt := r.composePrompt(template, template,
		SlackMessage{UserName: "alice"}, "do x", "https://slack/p", nil)
	if !strings.Contains(prompt, "Be concise.") {
		t.Error("expected user template at top")
	}
	if !strings.Contains(prompt, "Slack thread:") && !strings.Contains(prompt, "Request from") {
		t.Error("expected Slack context block to be appended for non-templated systems")
	}
}

// --- runner end-to-end ---

type fakeUtility struct {
	agents map[string]*utilitymodels.UtilityAgent
}

func (f fakeUtility) GetAgentByID(ctx context.Context, id string) (*utilitymodels.UtilityAgent, error) {
	a, ok := f.agents[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (f fakeUtility) PreparePromptRequest(
	ctx context.Context,
	utilityID string,
	tmplCtx *template.Context,
	defaults *utilityservice.DefaultUtilitySettings,
	sessionless bool,
) (*utilityservice.PromptRequest, error) {
	a, ok := f.agents[utilityID]
	if !ok {
		return nil, errors.New("not found")
	}
	// Resolve via the real engine so Custom vars actually substitute.
	resolved, err := template.NewEngine().ResolveWithOptions(a.Prompt, tmplCtx, template.ResolveOptions{MissingAsEmpty: sessionless})
	if err != nil {
		return nil, err
	}
	agentCLI, model := a.AgentID, a.Model
	if agentCLI == "" && defaults != nil {
		agentCLI = defaults.AgentID
	}
	if model == "" && defaults != nil {
		model = defaults.Model
	}
	return &utilityservice.PromptRequest{
		UtilityID:      utilityID,
		ResolvedPrompt: resolved,
		AgentCLI:       agentCLI,
		Model:          model,
	}, nil
}

type fakeUserDefaults struct {
	agentID, model string
	err            error
}

func (f fakeUserDefaults) GetDefaultUtilitySettings(ctx context.Context) (string, string, error) {
	return f.agentID, f.model, f.err
}

type fakeHostRunner struct {
	mu       sync.Mutex
	calls    []hostCall
	response string
	err      error
}

type hostCall struct {
	agentType, model, mode, prompt string
	mcpServers                     []agentctlutil.MCPServerDTO
}

func (f *fakeHostRunner) ExecutePromptWithMCP(
	ctx context.Context,
	agentType, model, mode, prompt string,
	mcpServers []agentctlutil.MCPServerDTO,
) (HostPromptResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, hostCall{agentType, model, mode, prompt, mcpServers})
	if f.err != nil {
		return HostPromptResult{}, f.err
	}
	return HostPromptResult{Response: f.response, Model: model, DurationMs: 1}, nil
}

func TestRunner_RunForMatch_HappyPath(t *testing.T) {
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"ua-1": {
			ID: "ua-1", Name: "Slack triage", AgentID: "claude-acp",
			Model: "claude-haiku-4-5", Enabled: true,
			Prompt: "You are a Slack triage helper.",
		},
	}}
	host := &fakeHostRunner{response: "Created task 'Fix checkout bug' in workspace ws-1."}
	mcp := []MCPDescriptor{{Name: "kandev", URL: "http://localhost:38429/mcp"}}
	r := NewRunner(utility, fakeUserDefaults{}, host, mcp, logger.Default())

	cfg := &SlackConfig{UtilityAgentID: "ua-1"}
	msg := SlackMessage{TS: "100.0001", ChannelID: "C1", UserID: "U1", UserName: "alice", Text: "!kandev fix checkout"}
	thread := []SlackMessage{msg}

	reply, err := r.RunForMatch(context.Background(), cfg, msg, "fix checkout", "https://example/p100", thread)
	if err != nil {
		t.Fatalf("RunForMatch: %v", err)
	}
	if !strings.Contains(reply, "Created task") {
		t.Errorf("unexpected reply: %q", reply)
	}
	if len(host.calls) != 1 {
		t.Fatalf("expected one host call, got %d", len(host.calls))
	}
	call := host.calls[0]
	if call.agentType != "claude-acp" || call.model != "claude-haiku-4-5" {
		t.Errorf("expected agent_id+model passthrough, got %+v", call)
	}
	if !strings.Contains(call.prompt, "You are a Slack triage helper.") {
		t.Error("expected utility agent's custom system prompt to be used")
	}
	if !strings.Contains(call.prompt, "fix checkout") {
		t.Error("expected user instruction in prompt")
	}
	if len(call.mcpServers) != 1 || call.mcpServers[0].Name != "kandev" || call.mcpServers[0].URL != "http://localhost:38429/mcp" {
		t.Errorf("expected kandev MCP wired, got %+v", call.mcpServers)
	}
	if call.mcpServers[0].Type != "http" {
		t.Errorf("expected http transport, got %q", call.mcpServers[0].Type)
	}
}

func TestRunner_RunForMatch_NoUtilityAgent(t *testing.T) {
	r := NewRunner(fakeUtility{}, fakeUserDefaults{}, &fakeHostRunner{}, nil, logger.Default())
	cfg := &SlackConfig{}
	_, err := r.RunForMatch(context.Background(), cfg, SlackMessage{}, "x", "", nil)
	if !errors.Is(err, ErrNoUtilityAgent) {
		t.Errorf("expected ErrNoUtilityAgent, got %v", err)
	}
}

func TestRunner_RunForMatch_DisabledBuiltinFallsBackToDefaults(t *testing.T) {
	// Built-in utility agents ship with enabled=false and empty agent_id /
	// model — they're meant to use the user's default agent + model. The
	// runner shouldn't reject this configuration; PreparePromptRequest
	// substitutes the defaults at resolution time, matching how the rest of
	// Kandev runs utility agents.
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"builtin-x": {ID: "builtin-x", Name: "x", Builtin: true, Enabled: false, Prompt: ""},
	}}
	host := &fakeHostRunner{response: "ok"}
	r := NewRunner(utility, fakeUserDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
		host, nil, logger.Default())
	cfg := &SlackConfig{UtilityAgentID: "builtin-x"}
	if _, err := r.RunForMatch(context.Background(), cfg, SlackMessage{TS: "100"}, "x", "", nil); err != nil {
		t.Fatalf("expected disabled built-in to run with user defaults, got: %v", err)
	}
	if len(host.calls) != 1 || host.calls[0].agentType != "claude-acp" || host.calls[0].model != "claude-haiku-4-5" {
		t.Errorf("expected default agent_id+model used, got %+v", host.calls)
	}
}

func TestRunner_RunForMatch_NoAgentResolvedErrors(t *testing.T) {
	// When the chosen agent has no agent_id and there are no user defaults
	// either, we can't pick an inference subprocess to spawn — surface a
	// clear error rather than silently failing.
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"ua-1": {ID: "ua-1", Name: "no-config", Builtin: false, Enabled: false},
	}}
	r := NewRunner(utility, fakeUserDefaults{}, &fakeHostRunner{}, nil, logger.Default())
	cfg := &SlackConfig{UtilityAgentID: "ua-1"}
	_, err := r.RunForMatch(context.Background(), cfg, SlackMessage{}, "x", "", nil)
	if err == nil || !strings.Contains(err.Error(), "no agent_id resolved") {
		t.Errorf("expected 'no agent_id resolved' error, got %v", err)
	}
}

func TestRunner_RunForMatch_EmptyResponseFallsBack(t *testing.T) {
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"ua-1": {ID: "ua-1", Name: "x", AgentID: "claude-acp", Model: "x", Enabled: true},
	}}
	host := &fakeHostRunner{response: ""}
	r := NewRunner(utility, fakeUserDefaults{}, host, nil, logger.Default())
	cfg := &SlackConfig{UtilityAgentID: "ua-1"}
	reply, err := r.RunForMatch(context.Background(), cfg, SlackMessage{TS: "100"}, "x", "", nil)
	if err != nil {
		t.Fatalf("expected success with fallback reply, got error: %v", err)
	}
	if reply == "" {
		t.Error("expected fallback reply text, got empty string")
	}
}

func TestRunner_RunForMatch_HostErrorBubbles(t *testing.T) {
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"ua-1": {ID: "ua-1", Name: "x", AgentID: "claude-acp", Model: "x", Enabled: true},
	}}
	host := &fakeHostRunner{err: errors.New("subprocess crashed")}
	r := NewRunner(utility, fakeUserDefaults{}, host, nil, logger.Default())
	cfg := &SlackConfig{UtilityAgentID: "ua-1"}
	_, err := r.RunForMatch(context.Background(), cfg, SlackMessage{TS: "100"}, "x", "", nil)
	if err == nil || !strings.Contains(err.Error(), "subprocess crashed") {
		t.Errorf("expected host error to bubble, got %v", err)
	}
}

// --- trigger end-to-end with fakes ---

type fakeClient struct {
	mu        sync.Mutex
	thread    []SlackMessage
	permalink string
	posted    []postRecord
	reactions []reactionRecord
}

type postRecord struct{ channel, threadTS, text string }
type reactionRecord struct{ channel, ts, name string }

func (f *fakeClient) AuthTest(ctx context.Context) (*TestConnectionResult, error) {
	return &TestConnectionResult{OK: true, UserID: "U1", TeamID: "T1"}, nil
}
func (f *fakeClient) SearchMessages(ctx context.Context, query string) ([]SlackMessage, error) {
	return nil, nil
}
func (f *fakeClient) ConversationsReplies(ctx context.Context, channelID, threadTS, triggerTS string) ([]SlackMessage, error) {
	return f.thread, nil
}
func (f *fakeClient) ChatGetPermalink(ctx context.Context, channelID, ts string) (string, error) {
	return f.permalink, nil
}
func (f *fakeClient) ChatPostMessage(ctx context.Context, channelID, threadTS, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posted = append(f.posted, postRecord{channelID, threadTS, text})
	return nil
}
func (f *fakeClient) ReactionsAdd(ctx context.Context, channelID, ts, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reactions = append(f.reactions, reactionRecord{channelID, ts, name})
	return nil
}

func TestTrigger_HandleOne_PostsAgentReplyInThread(t *testing.T) {
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"ua-1": {ID: "ua-1", Name: "x", AgentID: "claude-acp", Model: "claude-haiku-4-5", Enabled: true},
	}}
	host := &fakeHostRunner{response: "Created task X."}

	store := newTestStore(t)
	factory := func(_ *SlackConfig, _ string, _ string) Client {
		return &fakeClient{thread: []SlackMessage{{TS: "100.0001", Text: "!kandev x"}}}
	}
	svc := NewService(store, nil, NewRunner(utility, fakeUserDefaults{}, host, nil, logger.Default()), factory, logger.Default())
	ctx := context.Background()
	_ = svc.Store().UpsertConfig(ctx, &SlackConfig{
		AuthMethod:     AuthMethodCookie,
		UtilityAgentID: "ua-1", CommandPrefix: "!kandev",
	})

	client := &fakeClient{thread: []SlackMessage{{TS: "100.0001", Text: "!kandev fix it"}}, permalink: "https://example/p100"}
	svc.client = client

	cfg, _ := svc.Store().GetConfig(ctx)
	trig := NewTrigger(svc, logger.Default())
	reply, err := trig.handleOne(ctx, cfg, "!kandev", SlackMessage{
		TS: "100.0001", ChannelID: "C1", Text: "!kandev fix it",
	}, client)
	if err != nil {
		t.Fatalf("handleOne: %v", err)
	}
	if reply != "Created task X." {
		t.Errorf("expected agent reply passthrough, got %q", reply)
	}
	if len(client.posted) != 1 {
		t.Fatalf("expected one Slack reply, got %d", len(client.posted))
	}
	if client.posted[0].text != "Created task X." || client.posted[0].channel != "C1" {
		t.Errorf("unexpected slack post: %+v", client.posted[0])
	}
	if client.posted[0].threadTS != "100.0001" {
		t.Errorf("expected thread anchor on the trigger ts, got %q", client.posted[0].threadTS)
	}
	if len(client.reactions) != 1 {
		t.Fatalf("expected one ack reaction, got %d", len(client.reactions))
	}
	r := client.reactions[0]
	if r.channel != "C1" || r.ts != "100.0001" || r.name != AcknowledgeReaction {
		t.Errorf("unexpected reaction: %+v", r)
	}
}

// --- helper assertions ---

func TestRunner_PromptIncludesAllSlackContext(t *testing.T) {
	// Sanity: run a single match and inspect the prompt the host saw to
	// confirm permalink + workspace + thread all made it through. Belt and
	// braces against future refactors of buildAgentPrompt.
	utility := fakeUtility{agents: map[string]*utilitymodels.UtilityAgent{
		"ua-1": {ID: "ua-1", Name: "x", AgentID: "claude-acp", Model: "x", Enabled: true},
	}}
	host := &fakeHostRunner{response: "ok"}
	r := NewRunner(utility, fakeUserDefaults{}, host, nil, logger.Default())
	cfg := &SlackConfig{UtilityAgentID: "ua-1"}
	msg := SlackMessage{TS: "200", UserName: "bob", Text: "!kandev investigate"}
	thread := []SlackMessage{msg, {TS: "199", UserName: "alice", Text: "saw a 500 on /api/foo"}}
	if _, err := r.RunForMatch(context.Background(), cfg, msg, "investigate", "https://slack/p200", thread); err != nil {
		t.Fatalf("RunForMatch: %v", err)
	}
	prompt := host.calls[0].prompt
	for _, want := range []string{"https://slack/p200", "investigate", "saw a 500", "@bob"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("expected %q in prompt; got:\n%s", want, prompt)
		}
	}
}

// --- watermark advance under partial failure ---

// failingRunner fails handleOne the Nth time it's called (1-indexed).
type failingRunner struct {
	mu      sync.Mutex
	calls   int
	failOn  int
	fakeErr error
}

func (f *failingRunner) RunForMatch(ctx context.Context, cfg *SlackConfig, msg SlackMessage, instruction, permalink string, thread []SlackMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == f.failOn {
		return "", f.fakeErr
	}
	return "ok " + msg.TS, nil
}

// TestTrigger_ProcessMatches_BreaksOnTransientError exercises the regression
// the previous "continue on error" loop introduced: in a batch [A, B, C]
// where B fails with a transient error and C would succeed, the watermark
// must advance only past A — not past C — so the next tick re-processes B
// and C. This is the core promise of the !kandev capture flow ("no message
// silently dropped") and easy to break with a refactor that reverts to
// `continue`.
func TestTrigger_ProcessMatches_BreaksOnTransientError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.UpsertConfig(ctx, &SlackConfig{

		AuthMethod:     AuthMethodCookie,
		UtilityAgentID: "ua-1",
		CommandPrefix:  "!kandev",
	})
	cfg, _ := store.GetConfig(ctx)

	runner := &failingRunner{failOn: 2, fakeErr: errors.New("transient agent failure")}
	factory := func(_ *SlackConfig, _ string, _ string) Client {
		return &fakeClient{}
	}
	svc := NewService(store, nil, runner, factory, logger.Default())
	client := &fakeClient{thread: []SlackMessage{{TS: "100.000001", Text: "!kandev a"}}}
	svc.client = client

	trig := NewTrigger(svc, logger.Default())
	matches := []SlackMessage{
		{TS: "100.000001", ChannelID: "C1", Text: "!kandev a"},
		{TS: "100.000002", ChannelID: "C1", Text: "!kandev b"},
		{TS: "100.000003", ChannelID: "C1", Text: "!kandev c"},
	}
	trig.processMatches(ctx, cfg, "!kandev", matches, client)

	got, _ := svc.Store().GetConfig(ctx)
	if got.LastSeenTS != "100.000001" {
		t.Errorf("expected watermark to advance only past A (the consecutive prefix of successes), got %q", got.LastSeenTS)
	}
	if runner.calls != 2 {
		t.Errorf("expected loop to break after first failure (2 calls: A success, B fail); got %d calls", runner.calls)
	}
}

// TestTrigger_ProcessMatches_NoUtilityAgentSkipsWithoutBreak verifies the
// other branch of the failure logic: ErrNoUtilityAgent is unrecoverable, so
// processing continues to subsequent matches and the watermark spans all
// observed messages. Previously this was the only configured behaviour and
// is the only one a production user can hit until they pick an agent.
func TestTrigger_ProcessMatches_NoUtilityAgentSkipsWithoutBreak(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.UpsertConfig(ctx, &SlackConfig{

		AuthMethod:    AuthMethodCookie,
		CommandPrefix: "!kandev",
		// Note: no UtilityAgentID — runner will return ErrNoUtilityAgent.
	})
	cfg, _ := store.GetConfig(ctx)

	// Track invocations directly so we can assert the loop *didn't* break
	// on ErrNoUtilityAgent (only transient errors should break the loop).
	var calls int
	var callsMu sync.Mutex
	noAgentRunner := agentRunnerFunc(func(ctx context.Context, _ *SlackConfig, msg SlackMessage, _, _ string, _ []SlackMessage) (string, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		return "", ErrNoUtilityAgent
	})

	factory := func(_ *SlackConfig, _ string, _ string) Client { return &fakeClient{} }
	svc := NewService(store, nil, noAgentRunner, factory, logger.Default())
	svc.client = &fakeClient{}

	trig := NewTrigger(svc, logger.Default())
	matches := []SlackMessage{
		{TS: "100.000001", ChannelID: "C1", Text: "!kandev a"},
		{TS: "100.000002", ChannelID: "C1", Text: "!kandev b"},
	}
	trig.processMatches(ctx, cfg, "!kandev", matches, &fakeClient{})

	got, _ := svc.Store().GetConfig(ctx)
	if got.LastSeenTS != "100.000002" {
		t.Errorf("expected watermark to advance past *all* matches under ErrNoUtilityAgent, got %q", got.LastSeenTS)
	}
	callsMu.Lock()
	finalCalls := calls
	callsMu.Unlock()
	if finalCalls != 2 {
		t.Errorf("expected runner to be invoked for every match (no break), got %d calls", finalCalls)
	}
}

// agentRunnerFunc adapts a function to the AgentRunner interface for tests.
type agentRunnerFunc func(ctx context.Context, cfg *SlackConfig, msg SlackMessage, instruction, permalink string, thread []SlackMessage) (string, error)

func (f agentRunnerFunc) RunForMatch(ctx context.Context, cfg *SlackConfig, msg SlackMessage, instruction, permalink string, thread []SlackMessage) (string, error) {
	return f(ctx, cfg, msg, instruction, permalink, thread)
}

// TestComposePrompt_GenericTemplateGetsResolvedSubstitution verifies the
// follow-up fix to the composePrompt heuristic: when a template uses only
// non-Slack placeholders (e.g. {{UserPrompt}}), the fallback path uses the
// *resolved* template text rather than the raw template, so the agent sees
// the substituted value instead of the literal {{UserPrompt}} placeholder.
func TestComposePrompt_GenericTemplateGetsResolvedSubstitution(t *testing.T) {
	r := &Runner{}
	rawTemplate := "Be brief. Task: {{UserPrompt}}"
	resolved := "Be brief. Task: fix checkout"
	prompt := r.composePrompt(rawTemplate, resolved,
		SlackMessage{UserName: "alice"}, "fix checkout", "https://slack/p", nil)
	if !strings.Contains(prompt, "Task: fix checkout") {
		t.Errorf("expected resolved {{UserPrompt}} in fallback path; got:\n%s", prompt)
	}
	if strings.Contains(prompt, "{{UserPrompt}}") {
		t.Errorf("expected no literal placeholder to leak through; got:\n%s", prompt)
	}
}

// --- install-wide cadence ---

func TestTrigger_DueForScan(t *testing.T) {
	trig := &Trigger{}
	if !trig.dueForScan(30) {
		t.Error("expected fresh trigger to be due immediately")
	}
	trig.markScanned()
	if trig.dueForScan(30) {
		t.Error("expected trigger to not be due immediately after scan")
	}

	// Backdate the marker to simulate enough elapsed time. Using a direct
	// field write here rather than time.Sleep keeps the test deterministic.
	trig.scannedMu.Lock()
	trig.lastScannedAt = time.Now().Add(-31 * time.Second)
	trig.scannedMu.Unlock()
	if !trig.dueForScan(30) {
		t.Error("expected trigger to be due again after 31s with 30s interval")
	}

	// Bogus interval (below floor) falls back to the default — guards against
	// a config row with 0 (or a corrupted row) holding the loop hostage.
	trig.scannedMu.Lock()
	trig.lastScannedAt = time.Now()
	trig.scannedMu.Unlock()
	if trig.dueForScan(0) {
		t.Error("expected zero interval to behave as default 30s, not as 'always due'")
	}
}
