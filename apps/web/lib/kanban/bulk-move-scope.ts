type ScopeTask = { id: string; workflowId: string };
type ScopeStep = { id: string };
type ScopeSnapshot = {
  workflowId: string;
  tasks: ScopeTask[];
  steps: ScopeStep[];
};
type ScopeBoard = {
  workflowId: string | null;
  tasks: ScopeTask[];
  steps: ScopeStep[];
};

function resolveTaskWorkflow(
  id: string,
  snapshots: Record<string, ScopeSnapshot>,
  board: ScopeBoard,
): string | null {
  const owners = new Set<string>();
  for (const [workflowId, snapshot] of Object.entries(snapshots)) {
    const matches = snapshot.tasks.filter((task) => task.id === id);
    if (matches.length > 1) return null;
    if (matches.length === 1) {
      if (snapshot.workflowId !== workflowId || matches[0].workflowId !== workflowId) return null;
      owners.add(workflowId);
    }
  }
  const boardMatches = board.tasks.filter((task) => task.id === id);
  if (boardMatches.length > 1) return null;
  if (boardMatches.length === 1) {
    if (!board.workflowId || boardMatches[0].workflowId !== board.workflowId) return null;
    owners.add(board.workflowId);
  }
  return owners.size === 1 ? [...owners][0] : null;
}

/** Resolve every selected task against the same, consistent workflow evidence. */
export function resolveBulkSelectionWorkflow(
  ids: readonly string[],
  snapshots: Record<string, ScopeSnapshot>,
  board: ScopeBoard,
): string | null {
  if (ids.length === 0) return null;
  let selectedWorkflow: string | null = null;
  for (const id of ids) {
    const owner = resolveTaskWorkflow(id, snapshots, board);
    if (!owner || (selectedWorkflow && selectedWorkflow !== owner)) return null;
    selectedWorkflow = owner;
  }
  return selectedWorkflow;
}

/** The target must resolve to the selection's workflow and be currently offered. */
export function isBulkMoveTargetInWorkflow(
  targetStepId: string,
  workflowId: string,
  snapshots: Record<string, ScopeSnapshot>,
  board: ScopeBoard,
  hiddenSteps: Record<string, string[]>,
): boolean {
  if ((hiddenSteps[workflowId] ?? []).includes(targetStepId)) return false;
  const owners = new Set<string>();
  for (const [id, snapshot] of Object.entries(snapshots)) {
    const matches = snapshot.steps.filter((step) => step.id === targetStepId);
    if (matches.length > 1 || (matches.length && snapshot.workflowId !== id)) return false;
    if (matches.length) owners.add(id);
  }
  const boardMatches = board.steps.filter((step) => step.id === targetStepId);
  if (boardMatches.length > 1 || (boardMatches.length && !board.workflowId)) return false;
  if (boardMatches.length && board.workflowId) owners.add(board.workflowId);
  return owners.size === 1 && owners.has(workflowId);
}
