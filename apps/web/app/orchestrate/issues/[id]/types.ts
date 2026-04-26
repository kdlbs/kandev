/**
 * Local issue types for orchestrate task detail.
 * These will be replaced by backend-generated types once Wave 3A lands.
 */

export type IssueStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "done"
  | "cancelled"
  | "blocked";

export type IssuePriority = "critical" | "high" | "medium" | "low";

export type IssueComment = {
  id: string;
  taskId: string;
  authorType: "user" | "agent";
  authorId: string;
  authorName: string;
  content: string;
  toolCalls?: ToolCallEntry[];
  status?: string;
  durationMs?: number;
  createdAt: string;
};

export type ToolCallEntry = {
  id: string;
  name: string;
  input?: string;
  output?: string;
};

export type IssueActivityEntry = {
  id: string;
  actorName: string;
  actionVerb: string;
  targetName?: string;
  createdAt: string;
};

export type TaskSessionState =
  | "RUNNING"
  | "WAITING_FOR_INPUT"
  | "COMPLETED"
  | "FAILED";

export type TaskSession = {
  id: string;
  agentName: string;
  agentRole: string;
  state: TaskSessionState;
  isPrimary: boolean;
};

export type Issue = {
  id: string;
  workspaceId: string;
  identifier: string;
  title: string;
  description?: string;
  status: IssueStatus;
  priority: IssuePriority;
  labels: string[];
  assigneeAgentInstanceId?: string;
  assigneeName?: string;
  projectId?: string;
  projectName?: string;
  projectColor?: string;
  parentId?: string;
  parentTitle?: string;
  parentIdentifier?: string;
  blockedBy: string[];
  blocking: string[];
  children: Array<{ id: string; identifier: string; title: string; status: IssueStatus }>;
  reviewers: string[];
  approvers: string[];
  createdBy: string;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
};
