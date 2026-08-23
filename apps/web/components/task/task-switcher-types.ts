import type { ForegroundActivity, TaskSessionState, TaskState } from "@/lib/types/http";
import type { GroupedSidebarList } from "@/lib/sidebar/apply-view";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import type { SidebarTaskRowPresentation } from "@/lib/state/slices/ui/sidebar-task-row-presentation";

export type StepDef = {
  id: string;
  title: string;
  color?: string;
  events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }> };
};

export type TaskLinkHandler = (taskId: string, taskTitle?: string) => void;

export type TaskSwitcherItem = {
  id: string;
  title: string;
  autopilot?: boolean;
  state?: TaskState;
  sessionState?: TaskSessionState;
  /** Task-level most-active-wins busy aggregate (ADR-0049) from the task record. */
  foregroundActivity?: ForegroundActivity | null;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
  description?: string;
  workflowId?: string;
  workflowName?: string;
  workflowStepId?: string;
  workflowStepTitle?: string;
  repositoryPath?: string;
  repositories?: string[];
  /** Persisted task-to-repository links used by host-owned plugin task actions. */
  repositoryLinks?: Array<{ repository_id: string; position?: number }>;
  diffStats?: { additions: number; deletions: number };
  comparisonUnavailable?: boolean;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  lastActivityAt?: string;
  createdAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  parentTaskTitle?: string;
  parentTaskId?: string;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  prInfo?: { number: number; state: string; aggregateState?: string };
  /** Number of prompts currently en-queued for this task (mail badge). */
  queuedCount?: number;
  /** Destination-resident WIP queue position, separate from queued prompts. */
  wipQueue?: WipQueueStatus;
  isPRReview?: boolean;
  isIssueWatch?: boolean;
  issueInfo?: { url: string; number: number };
  agentErrorMessage?: string | null;
};

export type TaskSwitcherProps = {
  grouped: GroupedSidebarList;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  collapsedGroupKeys?: string[];
  onToggleGroup?: (groupKey: string) => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
  onSelectTask: (taskId: string) => void;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string, opts?: { cascade?: boolean }) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onDetachTask?: (taskId: string) => void;
  onLinkPullRequest?: TaskLinkHandler;
  onLinkIssue?: TaskLinkHandler;
  onLinkMergeRequest?: TaskLinkHandler;
  onLinkJiraTicket?: TaskLinkHandler;
  onLinkLinearIssue?: TaskLinkHandler;
  onLinkSentryIssue?: TaskLinkHandler;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  /** Re-parent a task under another task (drag onto a nest drop zone). */
  onNestTask?: (taskId: string, parentTaskId: string) => void;
  pinnedTaskIds?: string[];
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  isArchiving?: boolean;
  isLoading?: boolean;
  loadError?: string | null;
  onRetryLoad?: () => void;
  retryLabel?: string;
  totalTaskCount?: number;
  showActivityTime?: boolean;
  taskRowPresentation?: SidebarTaskRowPresentation;
  // Multi-select (cmd/shift click). When the selection is non-empty, plain
  // clicks toggle instead of navigating; the context menu acts on the selection.
  selectedTaskIds?: Set<string>;
  onToggleSelectTask?: (taskId: string) => void;
  onSelectTaskRange?: (taskId: string) => void;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkPin?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  onClearSelection?: () => void;
  isMixedWorkflowSelection?: boolean;
};
