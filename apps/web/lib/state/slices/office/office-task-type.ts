import type { OfficeTaskPriority, OfficeTaskStatus, TaskLabel } from "./types";
import type { TaskRepository } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

export type OfficeTask = {
  id: string;
  workspaceId: string;
  identifier: string;
  title: string;
  description?: string;
  status: OfficeTaskStatus;
  // Pre-normalization backend value (e.g. "SCHEDULING"); read only for a raw
  // sub-state the canonical union erases (ExecutionIndicator's "Live" dot).
  rawStatus?: string;
  priority: OfficeTaskPriority;
  parentId?: string;
  projectId?: string;
  assigneeAgentProfileId?: string;
  /**
   * The human assignee's user id. Independent of the agent assignee above:
   * a task can carry both, and setting one never clears the other.
   */
  assigneeUserId?: string;
  labels?: TaskLabel[] | string[];
  blockedBy?: string[];
  children?: OfficeTask[];
  executionPolicy?: string;
  executionState?: string;
  // Derived best-effort from the status-change activity log; not a column.
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
  // True when the task lives in a kandev-managed system workflow
  // (today: standing coordination; future: routine-fired). The Tasks
  // UI renders a "System" badge for these when the dev toggle reveals
  // them.
  isSystem?: boolean;
  statusSummary?: TaskStatusSummary | null;
  repositories?: TaskRepository[];
};
