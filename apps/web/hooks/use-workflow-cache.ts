import { useCallback, useRef, useSyncExternalStore } from "react";
import { useQueryClient, type QueryClient, type QueryKey } from "@tanstack/react-query";
import type { Workflow } from "@/lib/types/http";
import type { WorkflowItem } from "@/lib/state/slices";

export type WorkflowsByWorkspace = Record<string, WorkflowItem[]>;

const EMPTY_BY_WORKSPACE: WorkflowsByWorkspace = {};
const EMPTY_WORKFLOWS: WorkflowItem[] = [];

type WorkflowCacheSnapshot = {
  signature: string;
  workflowsByWorkspace: WorkflowsByWorkspace;
};

export function mapWorkflowItem(workflow: Workflow | WorkflowItem): WorkflowItem {
  if ("workspaceId" in workflow) {
    return workflow;
  }
  return {
    id: workflow.id,
    workspaceId: workflow.workspace_id,
    name: workflow.name,
    description: workflow.description ?? null,
    sortOrder: workflow.sort_order ?? 0,
    ...(workflow.agent_profile_id ? { agent_profile_id: workflow.agent_profile_id } : {}),
    ...(workflow.hidden !== undefined ? { hidden: workflow.hidden } : {}),
    ...(workflow.style !== undefined ? { style: workflow.style } : {}),
  };
}

export function useWorkflowsByWorkspace(): WorkflowsByWorkspace {
  const queryClient = useQueryClient();
  const snapshotRef = useRef<WorkflowCacheSnapshot>({
    signature: "",
    workflowsByWorkspace: EMPTY_BY_WORKSPACE,
  });
  const getSnapshot = useCallback(() => {
    const snapshot = readWorkflowsByWorkspace(queryClient);
    if (snapshot.signature === snapshotRef.current.signature) {
      return snapshotRef.current.workflowsByWorkspace;
    }
    snapshotRef.current = snapshot;
    return snapshot.workflowsByWorkspace;
  }, [queryClient]);

  return useSyncExternalStore(
    (onStoreChange) => queryClient.getQueryCache().subscribe(onStoreChange),
    getSnapshot,
    () => EMPTY_BY_WORKSPACE,
  );
}

export function useCachedWorkflows(workspaceId: string | null | undefined): WorkflowItem[] {
  const workflowsByWorkspace = useWorkflowsByWorkspace();
  if (!workspaceId) return EMPTY_WORKFLOWS;
  return workflowsByWorkspace[workspaceId] ?? EMPTY_WORKFLOWS;
}

export function useAllCachedWorkflows(): WorkflowItem[] {
  return Object.values(useWorkflowsByWorkspace()).flat();
}

export function readWorkflowsByWorkspace(queryClient: QueryClient): WorkflowCacheSnapshot {
  const queries = queryClient
    .getQueryCache()
    .findAll()
    .filter((query) => isWorkflowListQuery(query.queryKey) && Array.isArray(query.state.data))
    .sort((a, b) => a.state.dataUpdatedAt - b.state.dataUpdatedAt);

  const grouped = new Map<string, Map<string, WorkflowItem>>();
  for (const query of queries) {
    const workspaceId = query.queryKey[1] as string;
    const workspaceWorkflows = grouped.get(workspaceId) ?? new Map<string, WorkflowItem>();
    for (const workflow of query.state.data as Array<Workflow | WorkflowItem>) {
      const item = mapWorkflowItem(workflow);
      workspaceWorkflows.set(item.id, {
        ...workspaceWorkflows.get(item.id),
        ...item,
      });
    }
    grouped.set(workspaceId, workspaceWorkflows);
  }

  const workflowsByWorkspace = Object.fromEntries(
    [...grouped.entries()].map(([workspaceId, workflows]) => [
      workspaceId,
      [...workflows.values()],
    ]),
  );

  return {
    signature: queries
      .map(
        (query) => `${query.queryHash}:${query.state.dataUpdatedAt}:${query.state.dataUpdateCount}`,
      )
      .join("|"),
    workflowsByWorkspace,
  };
}

export function reorderCachedWorkflows(
  queryClient: QueryClient,
  workspaceId: string,
  workflowIds: string[],
) {
  const queries = queryClient
    .getQueryCache()
    .findAll()
    .filter((query) => isWorkflowListQuery(query.queryKey) && query.queryKey[1] === workspaceId);

  for (const query of queries) {
    queryClient.setQueryData(query.queryKey, (current: unknown) => {
      if (!Array.isArray(current)) return current;
      const byId = new Map(
        current
          .map((workflow) => [readWorkflowId(workflow), workflow] as const)
          .filter((entry): entry is readonly [string, unknown] => Boolean(entry[0])),
      );
      const reordered = workflowIds
        .map((id) => byId.get(id))
        .filter((workflow): workflow is NonNullable<typeof workflow> => workflow != null);
      for (const workflow of current) {
        const id = readWorkflowId(workflow);
        if (!id || workflowIds.includes(id)) continue;
        reordered.push(workflow);
      }
      return reordered;
    });
  }
}

function readWorkflowId(workflow: unknown): string | null {
  if (!workflow || typeof workflow !== "object" || !("id" in workflow)) return null;
  const id = workflow.id;
  return typeof id === "string" ? id : null;
}

export function isWorkflowListQuery(key: QueryKey): boolean {
  return (
    Array.isArray(key) &&
    key[0] === "workflows" &&
    typeof key[1] === "string" &&
    key.length === 3 &&
    typeof key[2] === "object" &&
    key[2] !== null &&
    "includeHidden" in key[2]
  );
}
