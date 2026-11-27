import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { recordTaskOverviewChange } from "@/lib/state/slices/task-overview-merge";
import { sidebarTaskPageCache } from "./sidebar-task-page-cache";

export function confirmSidebarDeletion(
  store: StoreApi<AppState>,
  taskIds: ReadonlySet<string>,
  workspaceId: string | null,
) {
  if (taskIds.size === 0 || store.getState().workspaces.activeId !== workspaceId) return;
  store.setState((state) => ({
    ...state,
    taskOverview: [...taskIds].reduce(
      (overview, id) => recordTaskOverviewChange(overview, id, null),
      state.taskOverview,
    ),
  }));
  sidebarTaskPageCache(store).removeTasks(taskIds);
}
