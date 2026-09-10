import type { AppState } from "@/lib/state/store";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

export type TaskMenuIdentity = Readonly<{ taskId: string; workspaceId: string }>;

export function resolveTaskMenuTarget(
  state: AppState,
  identity: TaskMenuIdentity,
): TaskSwitcherItem | null {
  if (state.workspaces.activeId !== identity.workspaceId) return null;
  const task = findTaskInSnapshots(
    identity.taskId,
    state.kanbanMulti.snapshots,
    state.kanban.tasks,
  );
  if (!task) return null;
  const workflow = state.workflows.items.find(
    (item) => item.id === task.workflowId && item.workspaceId === identity.workspaceId,
  );
  if (!workflow || (task.workspaceId && task.workspaceId !== identity.workspaceId)) return null;
  return {
    id: task.id,
    title: task.title,
    priority: task.priority,
    state: task.state,
    foregroundActivity: task.foregroundActivity,
    workflowId: task.workflowId,
    workflowStepId: task.workflowStepId,
    workspaceId: identity.workspaceId,
    repositoryLinks: task.repositories,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    primarySessionId: task.primarySessionId,
    parentTaskId: task.parentTaskId ?? undefined,
  };
}
