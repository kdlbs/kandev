import type { AppState } from "@/lib/state/store";
import type { WorkflowItem } from "@/lib/state/slices";

type KanbanRouteHydrationState = Pick<AppState, "workflows" | "workspaces">;

export type KanbanRouteSelection = {
  workspaceId?: string;
  workflowId?: string;
};

export function hasHydratedKanbanRouteState(
  state: KanbanRouteHydrationState,
  route: KanbanRouteSelection,
  workflows: WorkflowItem[],
  hydratedWorkflowIds: ReadonlySet<string>,
): boolean {
  const activeWorkspaceId = state.workspaces.activeId;
  if (!activeWorkspaceId) return false;
  if (route.workspaceId && route.workspaceId !== activeWorkspaceId) return false;

  const workspaceWorkflows = workflows.filter(
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
  return hydratedWorkflowIds.has(workflowId);
}
