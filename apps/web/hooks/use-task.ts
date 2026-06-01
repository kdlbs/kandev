import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

export function useTask(taskId: string | null) {
  // Reads from the TanStack Query `qk.kanban.multi()` cache (single source of
  // truth, populated by the kanban bridge + SSR seed).
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  const task = useMemo(() => {
    if (!taskId || !data) return null;
    return findTaskInSnapshots(taskId, data.snapshots) ?? null;
  }, [taskId, data]);

  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribe(taskId);
    return () => {
      unsubscribe();
    };
  }, [taskId]);

  return task;
}
