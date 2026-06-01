import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTaskSessionsByTask } from "@/hooks/domains/session/use-task-session-by-id";
import { taskSessionsQueryOptions } from "@/lib/query/query-options/session";

export function useTaskSessions(taskId: string | null) {
  const queryClient = useQueryClient();
  const { sessions, isLoading, isLoaded } = useTaskSessionsByTask(taskId);

  // Force a refetch of the per-task session list. The TQ query owns the fetch
  // (the byTask queryFn calls listTaskSessions); refetchQueries re-runs it and
  // the useTaskSessionsByTask hook seeds each session into its by-id slot.
  const loadSessions = useCallback(
    async (force = false) => {
      if (!taskId) return;
      const key = taskSessionsQueryOptions(taskId).queryKey;
      if (force) {
        await queryClient.refetchQueries({ queryKey: key });
      } else {
        await queryClient.ensureQueryData(taskSessionsQueryOptions(taskId));
      }
    },
    [queryClient, taskId],
  );

  return { sessions, isLoading, isLoaded, loadSessions };
}
