import type { AppState } from "@/lib/state/app-state-types";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { reconcileTaskOverviewRead } from "@/lib/state/slices/task-overview-merge";
import {
  matchesLocalSidebarTask,
  projectLocalSidebarTasks,
  sidebarCandidate,
} from "./sidebar-local-projection";

/** Page membership carries IDs; task objects are owned only by the canonical store. */
export function sidebarPageMembership(page: SidebarTaskPageResponse): SidebarTaskPageResponse {
  return {
    ...page,
    entries: page.entries.map(({ task, ...entry }) => ({
      ...entry,
      task_id: entry.task_id ?? task?.id,
      workflow_id: entry.workflow_id ?? task?.workflow_id,
      workflow_step_id: entry.workflow_step_id ?? task?.workflow_step_id,
    })),
  };
}

export function reconcileSidebarPage(
  state: AppState,
  page: SidebarTaskPageResponse,
  query: SidebarTaskQuery,
  readId: string,
) {
  const read = state.taskOverview.reads[readId];
  const incoming = page.entries.flatMap((entry) => (entry.task ? [toKanbanTask(entry.task)] : []));
  const reconciled = reconcileTaskOverviewRead(state.taskOverview, incoming, readId);
  if (!reconciled || !read) return null;
  const workspaceId = state.workspaces.activeId;
  if (reconciled.some((task) => task.workspaceId && task.workspaceId !== workspaceId)) return null;
  const candidates = reconciled.filter(sidebarCandidate);
  const projected = projectLocalSidebarTasks(candidates, state, workspaceId ?? "");
  const eligible = new Set(
    projected
      .filter(
        (task) =>
          !Object.hasOwn(read.changes, task.id) || matchesLocalSidebarTask(task, query.filters),
      )
      .map((task) => task.id),
  );
  const membership = sidebarPageMembership(page);
  membership.entries = membership.entries.filter(
    (entry) => entry.kind !== "task" || eligible.has(entry.task_id ?? ""),
  );
  return {
    page: membership,
    tasks: candidates.filter((task) => eligible.has(task.id)),
    provisional: Object.keys(read.changes).length > 0 || reconciled.length !== incoming.length,
  };
}
