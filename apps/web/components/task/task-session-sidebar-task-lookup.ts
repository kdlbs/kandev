import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { AppState } from "@/lib/state/app-state-types";
import type { AggregatedSidebarTasks } from "./task-session-sidebar-aggregate";

type SidebarTaskLookupState = Pick<AppState, "kanbanMulti" | "kanban" | "sidebarArchivedTasks">;

export function findSidebarTask(
  state: SidebarTaskLookupState,
  taskId: string,
  pageTasks: AggregatedSidebarTasks["allTasks"],
) {
  const pageTask = pageTasks.find((task) => task.id === taskId);
  if (pageTask) return pageTask;
  const activeTask = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
  if (activeTask) return activeTask;
  for (const tasks of Object.values(state.sidebarArchivedTasks.itemsByWorkspaceId)) {
    const archivedTask = tasks.find((task) => task.id === taskId);
    if (archivedTask) return archivedTask;
  }
  return undefined;
}
