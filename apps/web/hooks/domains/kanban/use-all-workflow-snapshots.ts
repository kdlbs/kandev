import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { useCachedWorkflows } from "@/hooks/use-workflow-cache";
import { workflowSnapshotToKanbanData } from "@/lib/kanban/snapshot";
import { workflowSnapshotQueryOptions } from "@/lib/query/query-options";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

export type AllWorkflowSnapshotsResult = {
  snapshots: Record<string, WorkflowSnapshotData>;
  isLoading: boolean;
};

export function useAllWorkflowSnapshots(workspaceId: string | null): AllWorkflowSnapshotsResult {
  const workflows = useCachedWorkflows(workspaceId);
  const workspaceWorkflows = useMemo(
    () => (workspaceId ? workflows : []),
    [workflows, workspaceId],
  );
  const queries = useQueries({
    queries: workspaceWorkflows.map((workflow) => ({
      ...workflowSnapshotQueryOptions(workflow.id),
      meta: { workflowName: workflow.name },
    })),
  });

  const snapshots = useMemo<Record<string, WorkflowSnapshotData>>(() => {
    const result: Record<string, WorkflowSnapshotData> = {};
    for (let index = 0; index < workspaceWorkflows.length; index++) {
      const workflow = workspaceWorkflows[index];
      const data = queries[index]?.data;
      if (!workflow || !data) continue;
      result[workflow.id] = workflowSnapshotToKanbanData(data, result[workflow.id]);
    }
    return result;
  }, [queries, workspaceWorkflows]);

  return {
    snapshots,
    isLoading: queries.some((query) => query.isFetching) && Object.keys(snapshots).length === 0,
  };
}
