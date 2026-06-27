import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { mapWorkflowItem } from "@/hooks/use-workflow-cache";
import { workflowsQueryOptions } from "@/lib/query/query-options";

export function useWorkflows(workspaceId: string | null, enabled = true) {
  const query = useQuery({
    ...workflowsQueryOptions(workspaceId ?? "", { includeHidden: true }),
    enabled: enabled && Boolean(workspaceId),
  });
  const workflows = useMemo(
    () => (query.data ? query.data.map(mapWorkflowItem) : []),
    [query.data],
  );

  return { workflows, isLoading: query.isFetching && workflows.length === 0 };
}
