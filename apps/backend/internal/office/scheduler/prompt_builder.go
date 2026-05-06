package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"
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

	// Office task-handoffs phase 8 — cross-task context appended to every
	// run prompt. Document bodies are intentionally NOT included; the
	// agent must call get_task_document_kandev to fetch them.
	HandoffContext *HandoffPromptContext
}

// HandoffPromptContext bundles the cross-task context surfaced by the
// office task-handoffs feature into each run's system prompt. The fields
// mirror v1.TaskContext but trimmed to what the agent actually needs to
// know.
type HandoffPromptContext struct {
	ParentRef        string             // identifier + title (e.g. "KAN-12 Plan implementation"); empty for roots
	SiblingRefs      []string           // identifier + title per sibling
	BlockerRefs      []string           // identifier + title per active blocker
	BlockedByRefs    []string           // tasks this one is currently blocking
	AvailableDocs    []HandoffDocPrompt // documents on self/parent/siblings; bodies NOT included
	WorkspaceMode    string             // inherit_parent | new_workspace | shared_group | ""
	WorkspaceMembers []string           // identifier + title of every active group member (besides self)
	MaterializedPath string             // reference path so the agent can `cd` if needed
	MaterializedKind string             // plain_folder | single_repo | multi_repo | remote_environment
	WorkspaceStatus  string             // active | requires_configuration
}

// HandoffDocPrompt is the metadata-only document projection appended to
// the prompt's "Documents available" section. Fetching the body requires
// an explicit get_task_document_kandev call.
type HandoffDocPrompt struct {
	TaskRef string // identifier + title of the document's owning task
	Key     string
	Title   string
}

// ChildSummaryPrompt holds display data for a completed child task.
type ChildSummaryPrompt struct {
	Identifier  string
	Title       string
	State       string
	LastComment string
}

// BuildPrompt generates a structured prompt for a run reason. The
// handoff context (cross-task references + available document keys) is
// appended to every prompt so the agent always sees the surrounding
// task tree, regardless of which reason woke it.
func BuildPrompt(pc *PromptContext) string {
	body := buildPromptBody(pc)
	if section := buildHandoffSection(pc.HandoffContext); section != "" {
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "\n" + section
	}
	return body
}

func buildPromptBody(pc *PromptContext) string {
	switch pc.Reason {
	case RunReasonTaskAssigned:
		return buildTaskAssignedPrompt(pc)
	case RunReasonTaskComment:
		return buildTaskCommentPrompt(pc)
	case RunReasonTaskBlockersResolved:
		return buildBlockersResolvedPrompt(pc)
	case RunReasonTaskChildrenCompleted:
		return buildChildrenCompletedPrompt(pc)
	case RunReasonApprovalResolved:
		return buildApprovalResolvedPrompt(pc)
	case RunReasonHeartbeat:
		return buildHeartbeatPrompt(pc)
	case RunReasonBudgetAlert:
		return buildBudgetAlertPrompt(pc)
	case RunReasonAgentError:
		return buildAgentErrorPrompt(pc)
	default:
		return fmt.Sprintf("You have been woken for reason: %s.", pc.Reason)
	}
}

// buildHandoffSection renders the "Related tasks" / "Documents
// available" / "Workspace" block. Returns the empty string when the
// HandoffPromptContext is nil or carries no information so the prompt
// stays untouched for tasks without any handoff context.
//
// Document bodies are intentionally omitted; agents must call
// get_task_document_kandev to fetch a specific document.
func buildHandoffSection(hc *HandoffPromptContext) string {
	if hc == nil || handoffSectionEmpty(hc) {
		return ""
	}
	var b strings.Builder
	appendRelationsSubsection(&b, hc)
	appendDocumentsSubsection(&b, hc)
	appendWorkspaceSubsection(&b, hc)
	return strings.TrimRight(b.String(), "\n")
}

func appendRelationsSubsection(b *strings.Builder, hc *HandoffPromptContext) {
	if hc.ParentRef == "" && len(hc.SiblingRefs) == 0 && len(hc.BlockerRefs) == 0 && len(hc.BlockedByRefs) == 0 {
		return
	}
	b.WriteString("Related tasks:\n")
	if hc.ParentRef != "" {
		fmt.Fprintf(b, "- Parent: %s\n", hc.ParentRef)
	}
	for _, ref := range hc.SiblingRefs {
		fmt.Fprintf(b, "- Sibling: %s\n", ref)
	}
	for _, ref := range hc.BlockerRefs {
		fmt.Fprintf(b, "- Blocked by: %s\n", ref)
	}
	for _, ref := range hc.BlockedByRefs {
		fmt.Fprintf(b, "- Blocks: %s\n", ref)
	}
}

func appendDocumentsSubsection(b *strings.Builder, hc *HandoffPromptContext) {
	if len(hc.AvailableDocs) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("Documents available (fetch with get_task_document_kandev):\n")
	for _, d := range hc.AvailableDocs {
		title := d.Title
		if title == "" {
			title = d.Key
		}
		fmt.Fprintf(b, "- %s %s — %q\n", d.TaskRef, d.Key, title)
	}
}

func appendWorkspaceSubsection(b *strings.Builder, hc *HandoffPromptContext) {
	if hc.WorkspaceMode == "" && hc.MaterializedPath == "" && len(hc.WorkspaceMembers) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("Workspace:\n")
	if hc.WorkspaceMode != "" {
		fmt.Fprintf(b, "- mode: %s\n", hc.WorkspaceMode)
	}
	if len(hc.WorkspaceMembers) > 0 {
		fmt.Fprintf(b, "- shared with: %s\n", strings.Join(hc.WorkspaceMembers, ", "))
	}
	if hc.MaterializedPath != "" {
		fmt.Fprintf(b, "- path: %s\n", hc.MaterializedPath)
	}
	if hc.MaterializedKind != "" {
		fmt.Fprintf(b, "- kind: %s\n", hc.MaterializedKind)
	}
	if hc.WorkspaceStatus != "" && hc.WorkspaceStatus != "active" {
		fmt.Fprintf(b, "- status: %s\n", hc.WorkspaceStatus)
	}
}

func handoffSectionEmpty(hc *HandoffPromptContext) bool {
	return hc.ParentRef == "" &&
		len(hc.SiblingRefs) == 0 &&
		len(hc.BlockerRefs) == 0 &&
		len(hc.BlockedByRefs) == 0 &&
		len(hc.AvailableDocs) == 0 &&
		hc.WorkspaceMode == "" &&
		hc.MaterializedPath == "" &&
		len(hc.WorkspaceMembers) == 0
}

func buildTaskAssignedPrompt(pc *PromptContext) string {
	switch pc.StageType {
	case "review":
		return buildReviewStagePrompt(pc)
	case "ship":
		return buildShipStagePrompt(pc)
	case "work":
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
