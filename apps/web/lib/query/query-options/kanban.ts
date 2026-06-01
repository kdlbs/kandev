import { queryOptions } from "@tanstack/react-query";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api/domains/kanban-api";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { qk } from "@/lib/query/keys";
import type { ListWorkflowsResponse } from "@/lib/types/http";
import type {
  KanbanState,
  WorkflowSnapshotData,
  WorkflowsState,
} from "@/lib/state/slices/kanban/types";

/** Cache value for the workspace-scoped workflows list. */
export type WorkflowsListData = WorkflowsState["items"];

/** Maps an HTTP `ListWorkflowsResponse` into the cached workflow-list shape. */
export function workflowsFromResponse(response: ListWorkflowsResponse): WorkflowsListData {
  return response.workflows.map((workflow) => ({
    id: workflow.id,
    workspaceId: workflow.workspace_id,
    name: workflow.name,
    description: workflow.description,
    sortOrder: workflow.sort_order ?? 0,
    agent_profile_id: workflow.agent_profile_id,
    hidden: workflow.hidden,
    style: workflow.style,
  }));
}

type KanbanTask = KanbanState["tasks"][number];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function stepFromDTO(
  step: {
    id: string;
    name: string;
    color?: string;
    position?: number;
    events?: KanbanState["steps"][number]["events"];
    show_in_command_panel?: boolean;
    allow_manual_move?: boolean;
    prompt?: string;
    is_start_step?: boolean;
    agent_profile_id?: string;
    stage_type?: KanbanState["steps"][number]["stage_type"];
  },
  index: number,
): KanbanState["steps"][number] {
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
    select: (data: KanbanMultiData): WorkflowSnapshotData | undefined => data.snapshots[wfId],
  };
}

/**
 * queryOptions for a single snapshot (shorthand for `workflowKanbanQueryOptions`).
 * Alias used by hooks that receive a `wfId`.
 */
/**
 * queryOptions for the workspace-scoped list of workflows (server data:
 * name, hidden, style, sortOrder, agent_profile_id). The `multi()` query holds
 * the per-workflow task/step snapshots; this holds the workflow metadata list.
 * Includes hidden workflows so the settings UI can manage them.
 */
export function workflowsListQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.kanban.workflowsList(workspaceId),
    queryFn: async (): Promise<WorkflowsListData> => {
      const response = await listWorkflows(workspaceId, {
        cache: "no-store",
        includeHidden: true,
      });
      return workflowsFromResponse(response);
    },
    enabled: !!workspaceId,
    staleTime: 30_000,
  });
}

export const kanbanQueryOptions = {
  /**
   * All workflow snapshots for the given workspace.
   *
   * Usage:
   *   useQuery(kanbanQueryOptions.multi(wsId))
   */
  multi: multiKanbanQueryOptions,

  /**
   * Workspace-scoped workflow list (metadata).
   *
   * Usage:
   *   useQuery(kanbanQueryOptions.workflowsList(wsId))
   */
  workflowsList: workflowsListQueryOptions,

  /**
   * Single workflow snapshot, derived from the multi() cache via select.
   *
   * Usage:
   *   useQuery(kanbanQueryOptions.workflow(wsId, wfId))
   */
  workflow: workflowKanbanQueryOptions,
};
