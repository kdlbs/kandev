import type { AppState } from "@/lib/state/store";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

export type TaskMenuIdentity = Readonly<{ taskId: string; workspaceId: string }>;

export function resolveTaskMenuTarget(
  state: AppState,
  identity: TaskMenuIdentity,
): TaskSwitcherItem | null {
  if (state.workspaces.activeId !== identity.workspaceId) return null;
  const taskLocation = findTaskLocation(state, identity.taskId);
  const task = taskLocation?.task;
  if (!task || task.isArchived) return null;
  const workflowId = task.workflowId ?? taskLocation.workflowId;
  if (!workflowId) return null;
  const workflow = state.workflows.items.find(
    (item) => item.id === workflowId && item.workspaceId === identity.workspaceId,
  );
  if (!workflow || (task.workspaceId && task.workspaceId !== identity.workspaceId)) return null;
  return {
    id: task.id,
    title: task.title,
    priority: task.priority,
    state: task.state,
    foregroundActivity: task.foregroundActivity,
    workflowId,
    workflowStepId: task.workflowStepId,
    workspaceId: identity.workspaceId,
    repositoryLinks: task.repositories,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    primarySessionId: task.primarySessionId,
    parentTaskId: task.parentTaskId ?? undefined,
    workspaceMode: task.workspaceMode,
  };
}

function findTaskLocation(state: AppState, taskId: string) {
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
    const task = snapshot.tasks.find((item) => item.id === taskId);
    if (task) return { task, workflowId: snapshot.workflowId || workflowId };
  }
  const task = findTaskInSnapshots(taskId, {}, state.kanban.tasks);
  return task ? { task, workflowId: task.workflowId } : null;
}
