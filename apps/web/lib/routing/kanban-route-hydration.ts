import type { AppState } from "@/lib/state/store";

type KanbanRouteHydrationState = Pick<
  AppState,
  "kanban" | "kanbanMulti" | "workflows" | "workspaces"
>;

export type KanbanRouteSelection = {
  workspaceId?: string;
  workflowId?: string;
};

export function hasHydratedKanbanRouteState(
  state: KanbanRouteHydrationState,
  route: KanbanRouteSelection,
): boolean {
  const activeWorkspaceId = state.workspaces.activeId;
  if (!activeWorkspaceId) return false;
  if (route.workspaceId && route.workspaceId !== activeWorkspaceId) return false;

  const workspaceWorkflows = state.workflows.items.filter(
    (workflow) => workflow.workspaceId === activeWorkspaceId,
  );
  if (workspaceWorkflows.length === 0) return false;
  if (
    route.workflowId &&
    !workspaceWorkflows.some((workflow) => workflow.id === route.workflowId)
  ) {
    return false;
  }

  const workflowId = route.workflowId ?? state.workflows.activeId;
  if (!workflowId) return false;
  return Boolean(state.kanbanMulti.snapshots[workflowId] || state.kanban.workflowId === workflowId);
}
