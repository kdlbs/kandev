package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	"github.com/kandev/kandev/internal/utility/template"
)

// agentSystemPrompt is the persona used when a utility agent's prompt template
// is empty (or doesn't reference any Slack-specific variables). It tells the
// agent it's acting as a triage assistant, names the MCP tools it has, and
// asks for a concise final reply suitable for posting back to Slack.
//
// When the user provides a custom prompt that references Slack template
// variables (e.g. {{SlackInstruction}}, {{SlackThread}}), the template wins
// and we don't append anything — the user is in full control.
const defaultSlackSystemPrompt = `You are a Kandev triage assistant. A user just sent a request from Slack — an instruction plus the surrounding thread for context. Your job:

1. Use the Kandev MCP tools available to you (list_workspaces_kandev, list_workflows_kandev, list_workflow_steps_kandev, list_repositories_kandev, create_task_kandev, list_agents_kandev, list_executor_profiles_kandev) to figure out which Kandev workspace + workflow + column + repository the request belongs in. **Always call list_workspaces_kandev first** — Slack is install-wide, so you must pick the destination Kandev workspace from the list rather than assume one. Then list_repositories_kandev for that workspace before deciding which repo to attach — never guess a repository_id.
2. Create a Kandev task that captures the request, with a clear title and a description that gives the future agent everything it needs (the user's instruction and any thread context that matters).
3. Reply briefly with what you did — the task title, the workspace it landed in, and a short rationale. Your reply will be posted back into Slack as your final message, so write naturally for a human reader.

Don't ask the user follow-up questions in the reply — your reply is posted in-thread without a chance for them to respond mid-run. If something is ambiguous, pick the most likely option and explain your choice.`

// HostUtilityRunner is the slice of hostutility.Manager we depend on.
type HostUtilityRunner interface {
	ExecutePromptWithMCP(
		ctx context.Context,
		agentType, model, mode, prompt string,
		mcpServers []agentctlutil.MCPServerDTO,
	) (HostPromptResult, error)
}

// HostPromptResult mirrors hostutility.PromptResult — duplicated here so the
// slack package doesn't import hostutility (which has heavy transitive deps).
type HostPromptResult struct {
	Response       string
	Model          string
	PromptTokens   int
	ResponseTokens int
	DurationMs     int
}

// UtilityRegistry is the slice of utility.Service the runner needs: enough to
// look up an agent and prepare a resolved prompt request that handles the
// "empty agent_id/model falls back to user defaults" path the rest of Kandev
// already implements.
type UtilityRegistry interface {
	GetAgentByID(ctx context.Context, id string) (*utilitymodels.UtilityAgent, error)
	PreparePromptRequest(
		ctx context.Context,
		utilityID string,
		tmplCtx *template.Context,
		defaults *utilityservice.DefaultUtilitySettings,
		sessionless bool,
	) (*utilityservice.PromptRequest, error)
}

// UserDefaults is the slice of user.Service the runner uses to resolve
// per-user default agent/model when the chosen utility agent leaves them
// empty (the normal state for built-in agents).
type UserDefaults interface {
	GetDefaultUtilitySettings(ctx context.Context) (agentID, model string, err error)
}

// MCPDescriptor describes a single MCP server to wire into the agent's
// session. Mirrors the agentctlutil.MCPServerDTO shape.
type MCPDescriptor struct {
	Name string
	URL  string
}

// Runner runs the utility agent for a Slack match.
type Runner struct {
	utility      UtilityRegistry
	userDefaults UserDefaults
	host         HostUtilityRunner
	mcpServers   []MCPDescriptor
	log          *logger.Logger
}

// NewRunner builds a runner.
func NewRunner(utility UtilityRegistry, userDefaults UserDefaults, host HostUtilityRunner, mcpServers []MCPDescriptor, log *logger.Logger) *Runner {
	return &Runner{utility: utility, userDefaults: userDefaults, host: host, mcpServers: mcpServers, log: log}
}

// ErrNoUtilityAgent is returned when the workspace's Slack config doesn't
// reference a utility agent.
var ErrNoUtilityAgent = errors.New("slack: no utility agent configured")

// RunForMatch implements AgentRunner. Slack is install-wide singleton, so
// there's no workspace_id to pass — the agent picks the destination Kandev
// workspace itself via list_workspaces_kandev.
func (r *Runner) RunForMatch(
	ctx context.Context,
	cfg *SlackConfig,
	msg SlackMessage,
	instruction, permalink string,
	thread []SlackMessage,
) (string, error) {
	if cfg.UtilityAgentID == "" {
		return "", ErrNoUtilityAgent
	}
	agent, err := r.utility.GetAgentByID(ctx, cfg.UtilityAgentID)
	if err != nil {
		return "", fmt.Errorf("load utility agent: %w", err)
	}

	defaults := r.resolveDefaults(ctx)
	tmplCtx := r.buildTemplateContext(msg, instruction, permalink, thread)

	prepared, err := r.utility.PreparePromptRequest(ctx, cfg.UtilityAgentID, tmplCtx, defaults, true)
	if err != nil {
		return "", fmt.Errorf("prepare prompt: %w", err)
	}
	if prepared.AgentCLI == "" {
		return "", fmt.Errorf("no agent_id resolved for utility agent %q (set one on the agent or pick a default in /settings/utility-agents)", agent.Name)
	}

	prompt := r.composePrompt(agent.Prompt, prepared.ResolvedPrompt, msg, instruction, permalink, thread)
	mcpDTOs := r.mcpDTOs()

	r.log.Info("slack: invoking utility agent",
		zap.String("utility_agent", agent.Name),
		zap.String("agent_cli", prepared.AgentCLI),
		zap.String("model", prepared.Model),
		zap.Int("mcp_servers", len(mcpDTOs)),
		zap.String("trigger_ts", msg.TS))

	res, err := r.host.ExecutePromptWithMCP(ctx, prepared.AgentCLI, prepared.Model, "", prompt, mcpDTOs)
	if err != nil {
		return "", fmt.Errorf("execute utility prompt: %w", err)
	}
	reply := strings.TrimSpace(res.Response)
	if reply == "" {
		r.log.Warn("slack: agent returned empty response",
			zap.String("trigger_ts", msg.TS))
		reply = "(Kandev triage agent finished without producing text — check the kanban board.)"
	}
	return reply, nil
}

// resolveDefaults reads the user-default agent/model. A read failure here
// just means we'll surface a clearer "no agent_id resolved" error later
// rather than crashing the whole flow on a settings-DB blip.
func (r *Runner) resolveDefaults(ctx context.Context) *utilityservice.DefaultUtilitySettings {
	if r.userDefaults == nil {
		return nil
	}
	agentID, model, err := r.userDefaults.GetDefaultUtilitySettings(ctx)
	if err != nil {
		r.log.Warn("slack: read default utility settings failed", zap.Error(err))
		return nil
	}
	return &utilityservice.DefaultUtilitySettings{AgentID: agentID, Model: model}
}

// buildTemplateContext maps Slack data into the existing template Context's
// Custom map so users can reference {{SlackInstruction}}, {{SlackThread}}, etc.
// in their utility agent's prompt template. UserPrompt is set too so prompts
// shared with other utility agents (which use {{UserPrompt}}) work without
// modification.
func (r *Runner) buildTemplateContext(trigger SlackMessage, instruction, permalink string, thread []SlackMessage) *template.Context {
	return &template.Context{
		UserPrompt: instruction,
		Custom: map[string]string{
			"SlackInstruction": instruction,
			"SlackThread":      formatThread(thread),
			"SlackPermalink":   permalink,
			"SlackUser":        senderLabel(trigger),
			"SlackChannelID":   trigger.ChannelID,
			"SlackTS":          trigger.TS,
		},
	}
}

// slackTemplateVars are the placeholders authored Slack-triage templates
// reference. Detecting any of these (not just "the resolver substituted
// anything") in the raw template is what tells us the user designed the
// prompt for Slack rather than reusing a generic utility-agent template
// that happens to use {{UserPrompt}}.
var slackTemplateVars = []string{
	"{{SlackInstruction}}",
	"{{SlackThread}}",
	"{{SlackPermalink}}",
	"{{SlackUser}}",
	"{{SlackChannelID}}",
	"{{SlackTS}}",
}

func referencesSlackVars(rawTemplate string) bool {
	for _, v := range slackTemplateVars {
		if strings.Contains(rawTemplate, v) {
			return true
		}
	}
	return false
}

// composePrompt picks between the resolved template (when the user authored
// one and it references at least one Slack-specific placeholder) and the
// legacy "default system prompt + appended Slack context block" layout.
//
// We test for Slack-specific placeholders explicitly, not just "the resolver
// substituted anything" — a generic utility-agent prompt that uses
// {{UserPrompt}} would otherwise hijack the Slack-owned path and the agent
// would lose the MCP-tool guidance baked into the default system prompt.
// When the raw template has no Slack vars, we fall back so users with empty
// or generic templates still get a working triage prompt.
func (r *Runner) composePrompt(rawTemplate, resolved string, trigger SlackMessage, instruction, permalink string, thread []SlackMessage) string {
	tmpl := strings.TrimSpace(rawTemplate)
	resolvedTrim := strings.TrimSpace(resolved)
	if tmpl != "" && referencesSlackVars(rawTemplate) {
		return resolvedTrim
	}
	system := resolvedTrim
	if system == "" {
		system = defaultSlackSystemPrompt
	}
	return appendSlackContextBlock(system, trigger, instruction, permalink, thread)
}

func appendSlackContextBlock(system string, trigger SlackMessage, instruction, permalink string, thread []SlackMessage) string {
	var b strings.Builder
	b.WriteString(system)
	b.WriteString("\n\n--- Slack context ---\n")
	if permalink != "" {
		b.WriteString("Slack thread: ")
		b.WriteString(permalink)
		b.WriteString("\n")
	}
	if instruction != "" {
		b.WriteString("Request from ")
		b.WriteString(senderLabel(trigger))
		b.WriteString(":\n")
		b.WriteString(instruction)
		b.WriteString("\n")
	}
	if len(thread) > 0 {
		b.WriteString("\nThread context:\n")
		b.WriteString(formatThread(thread))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatThread(thread []SlackMessage) string {
	if len(thread) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range thread {
		who := m.UserName
		if who == "" {
			who = m.UserID
		}
		if who == "" {
			who = "(unknown)"
		}
		b.WriteString(who)
		b.WriteString(" (")
		b.WriteString(m.TS)
		b.WriteString("): ")
		b.WriteString(strings.TrimSpace(m.Text))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r *Runner) mcpDTOs() []agentctlutil.MCPServerDTO {
	if len(r.mcpServers) == 0 {
		return nil
	}
	out := make([]agentctlutil.MCPServerDTO, 0, len(r.mcpServers))
	for _, m := range r.mcpServers {
		out = append(out, agentctlutil.MCPServerDTO{
			Name: m.Name,
			Type: "http",
			URL:  m.URL,
		})
	}
	return out
}

func senderLabel(m SlackMessage) string {
	switch {
	case m.UserName != "":
		return "@" + m.UserName
	case m.UserID != "":
		return "<@" + m.UserID + ">"
	default:
		return "the user"
	}
}
