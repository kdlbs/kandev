package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PromptContext holds the data needed to build a run prompt.
type PromptContext struct {
	Reason string

	// Task fields
	TaskID          string
	TaskIdentifier  string
	TaskTitle       string
	TaskDescription string
	TaskPriority    string
	ProjectName     string

	// Comment fields
	CommentBody       string
	CommentAuthor     string
	CommentAuthorType string

	// Blocker fields
	ResolvedBlockerTitles []string

	// Children completed fields
	ChildSummaries          []ChildSummaryPrompt
	ChildSummariesTruncated bool

	// Approval fields
	ApprovalType      string
	ApprovalStatus    string
	ApprovalNote      string
	ApprovalAgentName string

	// Heartbeat fields (CEO)
	AgentsIdle      int
	AgentsWorking   int
	AgentsPaused    int
	TasksInProgress int
	TasksCompleted  int
	TasksPending    int
	BudgetUsedPct   int
	RecentErrors    []string

	// Stage fields (execution policy)
	StageID         string   // execution policy stage ID
	StageType       string   // "work", "review", "approval", "ship"
	BuilderComments []string // latest comments from the builder/assignee
	ReviewFeedback  string   // aggregated feedback from rejected review

	// Runtime fields
	RunID          string
	AgentID        string
	SessionID      string
	TaskScope      []string
	AllowedActions []string

	// Office task-handoffs context. Populated by SchedulerIntegration
	// via the TaskContextProvider hook (HandoffService.GetTaskContext).
	// Document bodies are intentionally never inlined — the section
	// renders metadata + the fetch-tool name so the agent can call
	// get_task_document_kandev to read content.
	HandoffContext *v1.TaskContext
}

// BuildPrompt generates a structured prompt for a run reason.
func BuildPrompt(pc *PromptContext) string {
	var prompt string
	switch pc.Reason {
	case RunReasonTaskAssigned:
		prompt = buildTaskAssignedPrompt(pc)
	case RunReasonTaskComment:
		prompt = buildTaskCommentPrompt(pc)
	case RunReasonTaskBlockersResolved:
		prompt = buildBlockersResolvedPrompt(pc)
	case RunReasonTaskChildrenCompleted:
		prompt = buildChildrenCompletedPrompt(pc)
	case RunReasonApprovalResolved:
		prompt = buildApprovalResolvedPrompt(pc)
	case RunReasonHeartbeat:
		prompt = buildHeartbeatPrompt(pc)
	case RunReasonBudgetAlert:
		prompt = buildBudgetAlertPrompt(pc)
	case RunReasonAgentError:
		prompt = buildAgentErrorPrompt(pc)
	default:
		prompt = fmt.Sprintf("You have been woken for reason: %s.", pc.Reason)
	}
	prompt = appendHandoffSection(prompt, pc.HandoffContext)
	return appendRuntimeContext(prompt, pc)
}

// appendHandoffSection renders the office task-handoffs context block
// (Related tasks / Documents available / Workspace) onto every run
// prompt that carries a HandoffContext. Document bodies are
// intentionally NOT inlined — the section names the fetch tool so the
// agent can call get_task_document_kandev when it needs content.
//
// Returns the prompt unchanged when HandoffContext is nil or carries
// no relations / docs / workspace info.
func appendHandoffSection(prompt string, ctx *v1.TaskContext) string {
	if ctx == nil {
		return prompt
	}
	section := renderHandoffSection(ctx)
	if section == "" {
		return prompt
	}
	if !strings.HasSuffix(prompt, "\n") {
		prompt += "\n"
	}
	return prompt + "\n" + section
}

func renderHandoffSection(ctx *v1.TaskContext) string {
	var b strings.Builder
	appendRelationsBlock(&b, ctx)
	appendDocumentsBlock(&b, ctx)
	appendWorkspaceBlock(&b, ctx)
	return strings.TrimRight(b.String(), "\n")
}

func appendRelationsBlock(b *strings.Builder, ctx *v1.TaskContext) {
	if ctx.Parent == nil && len(ctx.Siblings) == 0 && len(ctx.Blockers) == 0 && len(ctx.BlockedBy) == 0 {
		return
	}
	b.WriteString("Related tasks:\n")
	if ctx.Parent != nil {
		fmt.Fprintf(b, "- Parent: %s\n", taskRefLabel(*ctx.Parent))
	}
	for _, t := range ctx.Siblings {
		fmt.Fprintf(b, "- Sibling: %s\n", taskRefLabel(t))
	}
	for _, t := range ctx.Blockers {
		fmt.Fprintf(b, "- Blocked by: %s\n", taskRefLabel(t))
	}
	for _, t := range ctx.BlockedBy {
		fmt.Fprintf(b, "- Blocks: %s\n", taskRefLabel(t))
	}
}

func appendDocumentsBlock(b *strings.Builder, ctx *v1.TaskContext) {
	if len(ctx.AvailableDocs) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("Documents available (fetch with get_task_document_kandev):\n")
	for _, d := range ctx.AvailableDocs {
		title := d.Title
		if title == "" {
			title = d.Key
		}
		fmt.Fprintf(b, "- %s %s — %q\n", taskRefLabel(d.TaskRef), d.Key, title)
	}
}

func appendWorkspaceBlock(b *strings.Builder, ctx *v1.TaskContext) {
	if ctx.WorkspaceGroup == nil && ctx.WorkspaceMode == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("Workspace:\n")
	if ctx.WorkspaceMode != "" {
		fmt.Fprintf(b, "- mode: %s\n", ctx.WorkspaceMode)
	}
	if g := ctx.WorkspaceGroup; g != nil {
		if len(g.Members) > 0 {
			labels := make([]string, 0, len(g.Members))
			for _, m := range g.Members {
				labels = append(labels, taskRefLabel(m))
			}
			fmt.Fprintf(b, "- shared with: %s\n", strings.Join(labels, ", "))
		}
		if g.MaterializedPath != "" {
			fmt.Fprintf(b, "- path: %s\n", g.MaterializedPath)
		}
		if g.MaterializedKind != "" {
			fmt.Fprintf(b, "- kind: %s\n", g.MaterializedKind)
		}
	}
	if ctx.WorkspaceStatus != "" && ctx.WorkspaceStatus != v1.TaskWorkspaceStatusActive {
		fmt.Fprintf(b, "- status: %s\n", ctx.WorkspaceStatus)
	}
}

func taskRefLabel(t v1.TaskRef) string {
	if t.Identifier != "" {
		return t.Identifier + " " + t.Title
	}
	return t.Title
}

func appendRuntimeContext(prompt string, pc *PromptContext) string {
	if pc == nil || pc.RunID == "" {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	fmt.Fprintf(&b, "\n\nRuntime context:\n- Run ID: %s\n- Agent ID: %s\n", pc.RunID, pc.AgentID)
	if pc.SessionID != "" {
		fmt.Fprintf(&b, "- Session ID: %s\n", pc.SessionID)
	}
	if len(pc.TaskScope) > 0 {
		fmt.Fprintf(&b, "- Task scope: %s\n", strings.Join(pc.TaskScope, ", "))
	}
	if len(pc.AllowedActions) > 0 {
		fmt.Fprintf(&b, "- Allowed actions: %s\n", strings.Join(pc.AllowedActions, ", "))
	}
	b.WriteString("Use runtime APIs for task, memory, skill, and agent mutations. If an action is denied, stop and explain the denial instead of bypassing it.")
	return b.String()
}

func buildTaskAssignedPrompt(pc *PromptContext) string {
	switch pc.StageType {
	case "review":
		return buildReviewStagePrompt(pc)
	case stageTypeShip:
		return buildShipStagePrompt(pc)
	case stageTypeWork:
		if pc.ReviewFeedback != "" {
			return buildReworkPrompt(pc)
		}
	}
	return buildDefaultWorkPrompt(pc)
}

func buildDefaultWorkPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You have been assigned task %s: %s.\n", taskRef(pc), pc.TaskTitle)
	if pc.ProjectName != "" {
		fmt.Fprintf(&b, "Project: %s\n", pc.ProjectName)
	}
	if pc.TaskDescription != "" {
		fmt.Fprintf(&b, "Description: %s\n", pc.TaskDescription)
	}
	if pc.TaskPriority != "" {
		fmt.Fprintf(&b, "Priority: %s\n", pc.TaskPriority)
	}
	b.WriteString("Read the description and start working.")
	return b.String()
}

func buildReviewStagePrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are reviewing task %s: %s.\n", taskRef(pc), pc.TaskTitle)
	if pc.TaskDescription != "" {
		fmt.Fprintf(&b, "\nTask description:\n%s\n", pc.TaskDescription)
	}
	if len(pc.BuilderComments) > 0 {
		fmt.Fprintf(&b, "\nBuilder's comments:\n%s\n", strings.Join(pc.BuilderComments, "\n"))
	}
	b.WriteString("\nReview the implementation carefully. Check for correctness, edge cases, and code quality.\n")
	b.WriteString("Submit your verdict: approve if the work is satisfactory, or reject with specific feedback on what needs to change.")
	return b.String()
}

func buildShipStagePrompt(pc *PromptContext) string {
	return fmt.Sprintf(
		"Task %s: %s has been approved by all reviewers.\n\nCommit the changes and create a pull request. Run verification (format, lint, test) before committing.",
		taskRef(pc), pc.TaskTitle,
	)
}

func buildReworkPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task %s: %s was returned by reviewers with feedback.\n", taskRef(pc), pc.TaskTitle)
	fmt.Fprintf(&b, "\nReviewer feedback:\n%s\n", pc.ReviewFeedback)
	b.WriteString("\nAddress the feedback and resubmit your changes.")
	return b.String()
}

func buildTaskCommentPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "New comment on your task %s: %s\n", taskRef(pc), pc.TaskTitle)
	if pc.CommentAuthor != "" {
		authorDesc := pc.CommentAuthor
		if pc.CommentAuthorType != "" {
			authorDesc += " (" + pc.CommentAuthorType + ")"
		}
		fmt.Fprintf(&b, "From: %s\n", authorDesc)
	}
	fmt.Fprintf(&b, "Comment: %s\n", pc.CommentBody)
	b.WriteString("Address this comment.")
	return b.String()
}

func buildBlockersResolvedPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "All blockers for your task %s: %s have been resolved.\n", taskRef(pc), pc.TaskTitle)
	if len(pc.ResolvedBlockerTitles) > 0 {
		fmt.Fprintf(&b, "Resolved blockers: %s\n", strings.Join(pc.ResolvedBlockerTitles, ", "))
	}
	b.WriteString("You can now proceed with your work.")
	return b.String()
}

// ChildSummaryPrompt holds display data for a completed child task.
type ChildSummaryPrompt struct {
	Identifier  string
	Title       string
	State       string
	LastComment string
}

func buildChildrenCompletedPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "All child tasks for your task %s: %s have completed.\n", taskRef(pc), pc.TaskTitle)

	if len(pc.ChildSummaries) > 0 {
		b.WriteString("\nCompleted children:\n")
		for _, c := range pc.ChildSummaries {
			writeChildSummaryLine(&b, &c)
		}
		if pc.ChildSummariesTruncated {
			b.WriteString("(showing first 20 children — fetch the full list via API)\n")
		}
	}

	b.WriteString("\nReview their output and determine next steps.")
	return b.String()
}

func writeChildSummaryLine(b *strings.Builder, c *ChildSummaryPrompt) {
	ref := c.Identifier
	if ref == "" {
		ref = "?"
	}
	fmt.Fprintf(b, "- %s (%s) [%s]", ref, c.Title, c.State)
	if c.LastComment != "" {
		summary := truncateComment(c.LastComment)
		fmt.Fprintf(b, " — %q", summary)
	}
	b.WriteString("\n")
}

func truncateComment(s string) string {
	if len(s) > 500 {
		return s[:485] + " [truncated]"
	}
	return s
}

func buildApprovalResolvedPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Your approval request %s has been %s.\n", pc.ApprovalType, pc.ApprovalStatus)
	if pc.ApprovalNote != "" {
		fmt.Fprintf(&b, "Decision note: %s\n", pc.ApprovalNote)
	}
	if pc.ApprovalStatus == "rejected" && pc.ApprovalType == "task_review" {
		b.WriteString("Address the feedback and resubmit.")
	}
	if pc.ApprovalStatus == "approved" && pc.ApprovalType == "hire_agent" && pc.ApprovalAgentName != "" {
		fmt.Fprintf(&b, "The new agent %s is now active.\n", pc.ApprovalAgentName)
	}
	return b.String()
}

func buildHeartbeatPrompt(pc *PromptContext) string {
	var b strings.Builder
	b.WriteString("Workspace status update:\n")
	fmt.Fprintf(&b, "Agents: %d idle, %d working, %d paused\n",
		pc.AgentsIdle, pc.AgentsWorking, pc.AgentsPaused)
	fmt.Fprintf(&b, "Tasks: %d in progress, %d completed since last heartbeat, %d pending assignment\n",
		pc.TasksInProgress, pc.TasksCompleted, pc.TasksPending)
	fmt.Fprintf(&b, "Budget: %d%% used this month\n", pc.BudgetUsedPct)
	if len(pc.RecentErrors) > 0 {
		fmt.Fprintf(&b, "Errors: %s\n", strings.Join(pc.RecentErrors, "; "))
	}
	b.WriteString("Review the status and take action if needed.")
	return b.String()
}

func buildBudgetAlertPrompt(pc *PromptContext) string {
	return fmt.Sprintf("Budget alert: %d%% of monthly budget has been used. Review spending.", pc.BudgetUsedPct)
}

func buildAgentErrorPrompt(pc *PromptContext) string {
	errMsg := "unknown"
	if len(pc.RecentErrors) > 0 {
		errMsg = pc.RecentErrors[0]
	}
	return fmt.Sprintf("An agent session has failed. Error: %s\nInvestigate and take corrective action.", errMsg)
}

func taskRef(pc *PromptContext) string {
	if pc.TaskIdentifier != "" {
		return "[" + pc.TaskIdentifier + "]"
	}
	return "[" + pc.TaskID + "]"
}

// ParseRunPayload extracts common fields from a run payload JSON.
func ParseRunPayload(payloadJSON string) map[string]string {
	result := make(map[string]string)
	if payloadJSON == "" || payloadJSON == "{}" {
		return result
	}
	_ = json.Unmarshal([]byte(payloadJSON), &result)
	return result
}

// continuationSummaryPromptCap is the byte slice of the continuation
// summary that gets prepended to a taskless run's prompt. 1,500 chars
// is small enough that a stale summary can't dominate the prompt,
// big enough to carry "## Active focus" + "## Open blockers" + a "## Next action" line.
const continuationSummaryPromptCap = 1500

// BuildAgentPromptResult bundles the assembled prompt plus the exact
// continuation summary slice that was prepended (empty when none).
// The caller is responsible for persisting both fields onto the runs
// row via UpdateRunPromptArtifacts.
type BuildAgentPromptResult struct {
	Prompt          string
	SummaryInjected string
}

// BuildAgentPrompt constructs the full prompt for an agent session.
// On fresh sessions, the agent's AGENTS.md content is prepended. On
// resume, only the wake context is included because the agent CLI
// retains instructions from the previous session.
//
// PR 1 of office-heartbeat-rework added taskless-run support: when
// taskID is empty AND continuationSummary is non-empty, the summary
// is prepended (sliced to 1,500 chars) before AGENTS.md so the agent
// has the "## Active focus" + "## Open blockers" + "## Next action"
// context without resuming a stale conversation. Today no caller
// passes taskID=="" so this branch is dead until PR 2 wires the
// agent_heartbeat cron.
//
// agentsMD is the AGENTS.md content read from the manifest (in-memory).
// Sibling references like `./HEARTBEAT.md` have already been rewritten
// to absolute paths under instructionsDir by resolveInstructionsForPrompt,
// so the prompt no longer needs a trailing "files live at <dir>" hint.
//
// Returns the assembled prompt + the verbatim slice of the continuation
// summary that was prepended (empty when no summary applied). The
// caller persists both onto the run row.
func (s *Service) BuildAgentPrompt(
	_ *models.Run,
	_ *models.AgentInstance,
	_, agentsMD string,
	isResume bool,
	wakeContext string,
	taskID, continuationSummary string,
) BuildAgentPromptResult {
	var sections []string

	summarySlice := ""
	if taskID == "" && continuationSummary != "" {
		summarySlice = sliceSummaryForPrompt(continuationSummary)
		if summarySlice != "" {
			sections = append(sections, summarySlice)
		}
	}

	if !isResume && agentsMD != "" {
		sections = append(sections, agentsMD)
	}

	if wakeContext != "" {
		sections = append(sections, wakeContext)
	}

	return BuildAgentPromptResult{
		Prompt:          strings.Join(sections, "\n\n---\n\n"),
		SummaryInjected: summarySlice,
	}
}

// sliceSummaryForPrompt clips the continuation-summary blob to the
// 1,500-char prompt cap. Cuts at a UTF-8 rune boundary so we don't
// emit a dangling continuation byte to the model.
func sliceSummaryForPrompt(s string) string {
	if len(s) <= continuationSummaryPromptCap {
		return s
	}
	cut := continuationSummaryPromptCap
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
