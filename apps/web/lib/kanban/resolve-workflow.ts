import type { WorkflowsState } from "@/lib/state/slices";

type WorkflowLike = { id: string; hidden?: boolean };

/**
 * Resolve the workflow id that should be active given the current store state
 * and persisted user settings.
 *
 * `null` is a valid "All Workflows" selection: when the user has explicitly
 * cleared the filter, we must not silently fall back to the first workflow.
 * Auto-selecting only happens when there is exactly one visible workflow —
 * otherwise the user would never be able to keep "All Workflows" picked.
 */
export function resolveDesiredWorkflowId({
  activeWorkflowId,
  settingsWorkflowId,
  workspaceWorkflows,
}: {
  activeWorkflowId?: string | null;
  settingsWorkflowId?: string | null;
  workspaceWorkflows: WorkflowsState["items"] | WorkflowLike[];
}): string | null {
  const visibleWorkflows = workspaceWorkflows.filter((workflow) => !workflow.hidden);
  if (activeWorkflowId && visibleWorkflows.some((workflow) => workflow.id === activeWorkflowId)) {
    return activeWorkflowId;
  }
  if (
    settingsWorkflowId &&
    visibleWorkflows.some((workflow) => workflow.id === settingsWorkflowId)
  ) {
    return settingsWorkflowId;
  }
  if (visibleWorkflows.length === 1) return visibleWorkflows[0].id;
  return null;
}
