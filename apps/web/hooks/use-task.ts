import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { taskQueryOptions } from "@/lib/query/query-options";

export function useTask(taskId: string | null) {
  const query = useQuery({
    ...taskQueryOptions(taskId ?? ""),
    enabled: Boolean(taskId),
  });

  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribe(taskId);
    return () => {
      unsubscribe();
    };
  }, [taskId]);

  return query.data ? toKanbanTask(query.data) : null;
}
