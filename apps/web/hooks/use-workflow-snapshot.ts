import { useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useOptionalAppStore } from "@/components/state-provider";
import { workflowSnapshotQueryOptions } from "@/lib/query/query-options";
import { workflowSnapshotToKanbanState } from "@/lib/kanban/snapshot";

export function useWorkflowSnapshot(workflowId: string | null) {
  const connectionStatus = useOptionalAppStore((state) => state.connection.status, "connected");
  const lastConnectedRef = useRef(connectionStatus === "connected");

  const query = useQuery({
    ...workflowSnapshotQueryOptions(workflowId ?? ""),
    enabled: Boolean(workflowId),
  });

  const snapshotState = useMemo(() => {
    if (query.data) return workflowSnapshotToKanbanState(query.data);
    return null;
  }, [query.data]);

  useEffect(() => {
    const wasConnected = lastConnectedRef.current;
    const isConnected = connectionStatus === "connected";
    lastConnectedRef.current = isConnected;
    const becameConnected = !wasConnected && isConnected;
    if (!workflowId || !becameConnected || !query.isFetched) return;
    void query.refetch();
    // query.refetch is stable for this observer; including the whole query
    // object would make the reconnect guard run on unrelated query updates.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connectionStatus, workflowId]);

  return { ...query, snapshotState };
}
