/**
 * Local task types for office task detail.
 * These will be replaced by backend-generated types once Wave 3A lands.
 */

export type TaskStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "done"
  | "cancelled"
  | "blocked";

export type TaskPriority = "critical" | "high" | "medium" | "low";

export type TaskRunStatus = "queued" | "claimed" | "finished" | "failed" | "cancelled";

export type TaskComment = {
  id: string;
  taskId: string;
  authorType: "user" | "agent";
  authorId: string;
  authorName: string;
  content: string;
  toolCalls?: ToolCallEntry[];
  status?: string;
  durationMs?: number;
  source?: string;
  createdAt: string;
  /**
   * Run lifecycle for the wakeup queued by this user comment's
   * comment_created subscriber. Absent for agent comments and for
   * user comments that didn't trigger a run (assignee paused, etc.).
   */
  runId?: string;
  runStatus?: TaskRunStatus;
  runError?: string;
};

export type TimelineEvent = {
  type: string;
  from?: string;
  to?: string;
  at: string;
};

export type ToolCallEntry = {
  id: string;
  name: string;
  input?: string;
  output?: string;
};

export type TaskActivityEntry = {
  id: string;
  actorType: "user" | "agent" | "system";
  actorId: string;
  actionVerb: string;
  targetName?: string;
  createdAt: string;
};

export type TaskSessionState =
  | "CREATED"
  | "STARTING"
  | "RUNNING"
  | "IDLE"
  | "WAITING_FOR_INPUT"
  | "COMPLETED"
  | "FAILED"
  | "CANCELLED";

export type TaskSession = {
  id: string;
  agentProfileId?: string;
  agentName: string;
  agentRole: string;
  state: TaskSessionState;
  isPrimary: boolean;
  startedAt?: string;
  completedAt?: string;
  updatedAt?: string;
  /** Verbatim error payload populated when state === "FAILED". */
  errorMessage?: string;
  /** Server-resolved tool_call count; powers the "ran N commands" segment
   *  of the timeline entry header without a per-session message fetch. */
  commandCount?: number;
};

/**
 * RunError is a chat-timeline view of a single FAILED office session.
 * Sourced from TaskSession (no separate API call) — derived in
 * task-chat.tsx for entries.kind === "error".
 */
export type RunError = {
  id: string;
  sessionId: string;
  agentProfileId?: string;
  rawPayload: string;
  failedAt: string;
};

export type TaskLabelLocal = {
  name: string;
  color: string;
};

export type TaskDecisionRole = "reviewer" | "approver";
export type TaskDecisionVerdict = "approved" | "changes_requested";

export type TaskDecision = {
  id: string;
  taskId: string;
  deciderType: "user" | "agent";
  deciderId: string;
  deciderName: string;
  role: TaskDecisionRole;
  decision: TaskDecisionVerdict;
  comment: string;
  createdAt: string;
};

export type Task = {
  id: string;
  workspaceId: string;
  identifier: string;
  title: string;
  description?: string;
  status: TaskStatus;
  priority: TaskPriority;
  labels: TaskLabelLocal[];
  assigneeAgentProfileId?: string;
  assigneeName?: string;
  projectId?: string;
  projectName?: string;
  projectColor?: string;
  parentId?: string;
  parentTitle?: string;
  parentIdentifier?: string;
  blockedBy: string[];
  blocking: string[];
  children: Array<{
    id: string;
    identifier: string;
    title: string;
    status: TaskStatus;
    blockedBy?: string[];
    createdAt?: string;
  }>;
  reviewers: string[];
  approvers: string[];
  decisions: TaskDecision[];
  createdBy: string;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
  executionPolicy?: string;
  executionState?: string;
};
