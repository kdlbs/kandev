import type { TaskWorkflowCoverage } from "@/lib/types/http";
import type { AppState } from "../app-state-types";

/** Live task changes can add scopes, while removals only leave harmless empty scopes. */
export function reconcileTaskWorkflowCoverage(
  state: AppState,
  coverage: TaskWorkflowCoverage | undefined,
  readId: string | undefined,
): TaskWorkflowCoverage | undefined {
  if (!coverage || !readId) return coverage;
  const read = state.taskOverview.reads[readId];
  if (!read || read.scope !== state.taskOverview.scope) return { ...coverage, complete: false };
  const ids = new Set(coverage.workflow_ids);
  for (const [id, change] of Object.entries(read.changes)) {
    if (!change) continue;
    const workflow = change.workflowId ?? state.taskOverview.byId[id]?.workflowId;
    if (workflow !== undefined) ids.add(workflow);
  }
  return { ...coverage, workflow_ids: [...ids] };
}
