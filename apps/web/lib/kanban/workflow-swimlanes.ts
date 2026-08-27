export type WorkflowLike = { id: string; name: string; hidden?: boolean };

/**
 * Selects the workflow swimlanes the board renders.
 *
 * - No filter ("All Workflows"): every workflow with a loaded snapshot that is
 *   not hidden (system/office workflows stay off the board by design).
 * - Explicit filter: the selected workflow itself, hidden or not. The display
 *   dropdown offers every workflow in the store — including hidden ones like
 *   Improve Kandev — so an explicit selection must render that board;
 *   resolving against the visible-only list would silently show "No tasks yet"
 *   for a workflow that has tasks.
 */
export function selectWorkflowSwimlanes(
  workflowFilter: string | null | undefined,
  workflows: WorkflowLike[],
  snapshots: Record<string, unknown>,
): WorkflowLike[] {
  if (workflowFilter) {
    const workflow = workflows.find((item) => item.id === workflowFilter && snapshots[item.id]);
    return workflow ? [workflow] : [];
  }
  return workflows.filter((workflow) => !workflow.hidden && snapshots[workflow.id]);
}

/**
 * Selects the workflows the mobile board navigator offers. The navigator is
 * the only workflow switcher on the mobile kanban page (the display menu hides
 * its workflow select there), so hidden workflows with tasks — e.g. Improve
 * Kandev, whose tasks land in a hidden workflow — must be reachable, mirroring
 * the sidebar which already aggregates their snapshots. A hidden workflow with
 * live hidden columns must also stay reachable so its columns can be restored.
 * Empty hidden system workflows stay out.
 */
export function selectMobileNavigatorWorkflows(
  visibleOrdered: WorkflowLike[],
  workflows: WorkflowLike[],
  getFilteredTasks: (workflowId: string) => unknown[],
  hasLiveHiddenSteps: (workflowId: string) => boolean = () => false,
): Array<{ workflow: WorkflowLike; tasks: unknown[] }> {
  const visibleIds = new Set(visibleOrdered.map((workflow) => workflow.id));
  const entries: Array<{ workflow: WorkflowLike; tasks: unknown[] }> = [];
  for (const workflow of visibleOrdered) {
    entries.push({ workflow, tasks: getFilteredTasks(workflow.id) });
  }
  for (const workflow of workflows) {
    if (!workflow.hidden || visibleIds.has(workflow.id)) continue;
    const tasks = getFilteredTasks(workflow.id);
    if (tasks.length > 0 || hasLiveHiddenSteps(workflow.id)) {
      entries.push({ workflow, tasks });
    }
  }
  return entries;
}

export type SelectVisibleWorkflowsInput = {
  workflowFilter: string | null;
  orderedWorkflows: WorkflowLike[];
  /** Whether the workflow has at least one task surviving every active filter. */
  hasTasks: (workflowId: string) => boolean;
  /**
   * Whether the workflow's hidden set intersects its live snapshot steps.
   *
   * A workflow whose columns the user hid must keep its lane, because the lane
   * header carries the Columns menu that is the only way to restore them. The
   * predicate is the LIVE intersection on purpose: a hidden id whose step no
   * longer exists hides nothing, so it must not hold a lane open either.
   */
  hasLiveHiddenSteps: (workflowId: string) => boolean;
  showEmptyBoard: boolean;
};

/**
 * Selects which workflows the board renders.
 *
 * An explicit workflow filter renders exactly what it selected. Otherwise a
 * workflow appears when it has visible tasks OR when it is holding hidden
 * columns, with the pre-existing all-workflows fallback for an empty mobile
 * board.
 */
export function selectVisibleWorkflows({
  workflowFilter,
  orderedWorkflows,
  hasTasks,
  hasLiveHiddenSteps,
  showEmptyBoard,
}: SelectVisibleWorkflowsInput): WorkflowLike[] {
  if (workflowFilter) return orderedWorkflows;
  const retained = orderedWorkflows.filter(
    (workflow) => hasTasks(workflow.id) || hasLiveHiddenSteps(workflow.id),
  );
  if (retained.length > 0 || !showEmptyBoard) return retained;
  return orderedWorkflows;
}
