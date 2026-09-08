import type { TaskSwitcherItem } from "./task-switcher";
import type { useArchivedTaskState } from "./task-archived-context";
import { t } from "@/lib/i18n";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { buildSidebarItem, type SidebarItemContext } from "./task-session-sidebar-item";

export function buildArchivedSidebarItem(
  s: ReturnType<typeof useArchivedTaskState>,
  context?: SidebarItemContext,
): TaskSwitcherItem {
  if (s.archivedTask && context) {
    const task = toKanbanTask(s.archivedTask);
    return buildSidebarItem({ ...task, _workflowId: task.workflowId }, context);
  }
  return {
    id: s.archivedTaskId!,
    title: s.archivedTaskTitle ?? t("task:archivedTask"),
    state: undefined,
    sessionState: undefined,
    description: undefined,
    workflowId: undefined,
    workflowName: undefined,
    workflowStepId: undefined,
    workflowStepTitle: undefined,
    repositoryPath: s.archivedTaskRepositoryLabel,
    diffStats: undefined,
    isRemoteExecutor: false,
    remoteExecutorType: undefined,
    remoteExecutorName: undefined,
    primarySessionId: null,
    hasPendingClarification: false,
    hasPendingPermission: false,
    updatedAt: s.archivedTaskUpdatedAt,
    createdAt: undefined,
    isArchived: true,
    parentTaskTitle: undefined,
    parentTaskId: undefined,
    prInfo: undefined,
    isPRReview: false,
    isIssueWatch: false,
    issueInfo: undefined,
    agentErrorMessage: null,
  };
}
