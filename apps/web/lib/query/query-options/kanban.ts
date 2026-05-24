import { queryOptions } from "@tanstack/react-query";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api/domains/kanban-api";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { qk } from "@/lib/query/keys";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function stepFromDTO(step: { id: string; name: string; color?: string; position?: number; events?: KanbanState["steps"][number]["events"]; show_in_command_panel?: boolean; allow_manual_move?: boolean; prompt?: string; is_start_step?: boolean; agent_profile_id?: string; stage_type?: KanbanState["steps"][number]["stage_type"] }, index: number): KanbanState["steps"][number] {
  return {
    id: step.id,
    title: step.name,
    color: step.color ?? "bg-neutral-400",
    position: step.position ?? index,
    events: step.events,
    show_in_command_panel: step.show_in_command_panel,
    allow_manual_move: step.allow_manual_move,
    prompt: step.prompt,
    is_start_step: step.is_start_step,
    agent_profile_id: step.agent_profile_id,
    stage_type: step.stage_type,
  };
}

export function snapshotToWorkflowSnapshotData(
  workflowId: string,
  workflowName: string,
  raw: Awaited<ReturnType<typeof fetchWorkflowSnapshot>>,
): WorkflowSnapshotData {
  const steps = raw.steps.map((s, idx) => stepFromDTO(s as Parameters<typeof stepFromDTO>[0], idx));
  const stepIds = new Set(steps.map((s) => s.id));
  const tasks: KanbanTask[] = raw.tasks
    .filter((t) => !t.is_ephemeral)
    .filter((t) => t.workflow_step_id && stepIds.has(t.workflow_step_id))
    .map((t) => toKanbanTask(t));

  return { workflowId, workflowName, steps, tasks };
}

// ---------------------------------------------------------------------------
// Multi-snapshot response type — single source of truth for the cache
// ---------------------------------------------------------------------------

export type KanbanMultiData = {
  /** workflowId → WorkflowSnapshotData */
  snapshots: Record<string, WorkflowSnapshotData>;
};

// ---------------------------------------------------------------------------
// queryOptions factories
// ---------------------------------------------------------------------------

/**
 * queryOptions for the multi-workflow snapshot view.
 *
 * Requires `workspaceId` to know which workflows to load. The cache key is
 * workspace-scoped so switching workspace fetches fresh data.
 *
 * SSR: pass the return value to `queryClient.prefetchQuery`.
 * CSR: pass to `useQuery(kanbanQueryOptions.multi(wsId))`.
 */
export function multiKanbanQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.kanban.multi(),
    queryFn: async (): Promise<KanbanMultiData> => {
      const { workflows } = await listWorkflows(workspaceId);
      const entries = await Promise.all(
        workflows.map(async (wf) => {
          const raw = await fetchWorkflowSnapshot(wf.id, { cache: "no-store" });
          return [wf.id, snapshotToWorkflowSnapshotData(wf.id, wf.name, raw)] as const;
        }),
      );
      return { snapshots: Object.fromEntries(entries) };
    },
    enabled: !!workspaceId,
    staleTime: 30_000,
  });
}

/**
 * Derives a single-workflow snapshot from the multi() cache via `select`.
 * Avoids duplicate network requests — the multi() cache is the source.
 */
export function workflowKanbanQueryOptions(workspaceId: string, wfId: string) {
  return {
    ...multiKanbanQueryOptions(workspaceId),
    select: (data: KanbanMultiData): WorkflowSnapshotData | undefined =>
      data.snapshots[wfId],
  };
}

/**
 * queryOptions for a single snapshot (shorthand for `workflowKanbanQueryOptions`).
 * Alias used by hooks that receive a `wfId`.
 */
export const kanbanQueryOptions = {
  /**
   * All workflow snapshots for the given workspace.
   *
   * Usage:
   *   useQuery(kanbanQueryOptions.multi(wsId))
   */
  multi: multiKanbanQueryOptions,

  /**
   * Single workflow snapshot, derived from the multi() cache via select.
   *
   * Usage:
   *   useQuery(kanbanQueryOptions.workflow(wsId, wfId))
   */
  workflow: workflowKanbanQueryOptions,

  /**
   * Single task, derived from a workflow snapshot via select.
   *
   * Usage:
   *   useQuery(kanbanQueryOptions.task(wsId, wfId, taskId))
   */
  task: (workspaceId: string, wfId: string, taskId: string) => ({
    ...multiKanbanQueryOptions(workspaceId),
    select: (data: KanbanMultiData): KanbanTask | undefined =>
      data.snapshots[wfId]?.tasks.find((t) => t.id === taskId),
    queryKey: qk.kanban.task(taskId),
  }),
};
