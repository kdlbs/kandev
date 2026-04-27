package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// PromptContext holds the data needed to build a wakeup prompt.
type PromptContext struct {
	Reason string

	// Task fields
	TaskID          string
	TaskIdentifier  string
	TaskTitle       string
	TaskDescription string
	TaskPriority    int
	ProjectName     string

	// Comment fields
	CommentBody       string
	CommentAuthor     string
	CommentAuthorType string

	// Blocker fields
	ResolvedBlockerTitles []string

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
}

// BuildPrompt generates a structured prompt for a wakeup reason.
func BuildPrompt(pc *PromptContext) string {
	switch pc.Reason {
	case WakeupReasonTaskAssigned:
		return buildTaskAssignedPrompt(pc)
	case WakeupReasonTaskComment:
		return buildTaskCommentPrompt(pc)
	case WakeupReasonTaskBlockersResolved:
		return buildBlockersResolvedPrompt(pc)
	case WakeupReasonTaskChildrenCompleted:
		return buildChildrenCompletedPrompt(pc)
	case WakeupReasonApprovalResolved:
		return buildApprovalResolvedPrompt(pc)
	case WakeupReasonHeartbeat:
		return buildHeartbeatPrompt(pc)
	case WakeupReasonBudgetAlert:
		return buildBudgetAlertPrompt(pc)
	case WakeupReasonAgentError:
		return buildAgentErrorPrompt(pc)
	default:
		return fmt.Sprintf("You have been woken for reason: %s.", pc.Reason)
	}
}

func buildTaskAssignedPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You have been assigned task %s: %s.\n", taskRef(pc), pc.TaskTitle)
	if pc.ProjectName != "" {
		fmt.Fprintf(&b, "Project: %s\n", pc.ProjectName)
	}
	if pc.TaskDescription != "" {
		fmt.Fprintf(&b, "Description: %s\n", pc.TaskDescription)
	}
	if pc.TaskPriority > 0 {
		fmt.Fprintf(&b, "Priority: %d\n", pc.TaskPriority)
	}
	b.WriteString("Read the description and start working.")
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
	return fmt.Sprintf(
		"All child tasks for your task %s: %s have completed.\nReview their output and determine next steps.",
		taskRef(pc), pc.TaskTitle,
	)
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

// ParseWakeupPayload extracts common fields from a wakeup payload JSON.
func ParseWakeupPayload(payloadJSON string) map[string]string {
	result := make(map[string]string)
	if payloadJSON == "" || payloadJSON == "{}" {
		return result
	}
	_ = json.Unmarshal([]byte(payloadJSON), &result)
	return result
}

// BuildAgentPrompt constructs the full prompt for an agent session.
// On fresh sessions, the agent's AGENTS.md content (with a path directive)
// is prepended. On resume, only the wake context is included because the
// agent CLI retains instructions from the previous session.
func (s *Service) BuildAgentPrompt(
	wakeup *models.WakeupRequest,
	_ *models.AgentInstance,
	instructionsDir string,
	isResume bool,
	wakeContext string,
) string {
	var sections []string

	if !isResume && instructionsDir != "" {
		content := readInstructionFile(
			filepath.Join(instructionsDir, "AGENTS.md"),
		)
		if content != "" {
			content += fmt.Sprintf(
				"\n\nThe above agent instructions were loaded from %s/AGENTS.md.\n"+
					"Resolve any relative file references from %s.\n"+
					"This directory contains sibling instruction files: "+
					"./HEARTBEAT.md, ./SOUL.md, ./TOOLS.md.\n"+
					"Read them when referenced in these instructions.",
				instructionsDir, instructionsDir,
			)
			sections = append(sections, content)
		}
	}

	if wakeContext != "" {
		sections = append(sections, wakeContext)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// readInstructionFile reads a file from disk, returning empty string on error.
func readInstructionFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
