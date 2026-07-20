import type { WorkflowsState } from "@/lib/state/slices";

type WorkflowLike = { id: string; hidden?: boolean };

/**
 * Resolve the workflow context used by board actions while a mobile workflow
 * selection is waiting for its single-workflow snapshot to hydrate.
 */
export function resolveBoardWorkflowId({
  isMobile,
  selectedWorkflowId,
  focusedWorkflowId,
  hydratedWorkflowId,
}: {
  isMobile: boolean;
  selectedWorkflowId: string | null;
  focusedWorkflowId: string | null;
  hydratedWorkflowId: string | null;
}): string | null {
  if (!isMobile) return hydratedWorkflowId;
  return selectedWorkflowId ?? focusedWorkflowId;
}

/**
 * Resolve steps for board actions without pairing a newly selected workflow
 * with steps left over from the previously hydrated workflow.
 */
export function resolveBoardWorkflowSteps<TStep>({
  effectiveWorkflowId,
  hydratedWorkflowId,
  snapshots,
  activeSteps,
}: {
  effectiveWorkflowId: string | null;
  hydratedWorkflowId: string | null;
  snapshots: Record<string, { steps: Array<TStep & { position: number }> } | undefined>;
  activeSteps: TStep[];
}): TStep[] {
  const effectiveSnapshot = effectiveWorkflowId ? snapshots[effectiveWorkflowId] : undefined;
  if (effectiveSnapshot) {
    return [...effectiveSnapshot.steps].sort((a, b) => a.position - b.position);
  }
  if (effectiveWorkflowId === hydratedWorkflowId) return activeSteps;
  return [];
}

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
