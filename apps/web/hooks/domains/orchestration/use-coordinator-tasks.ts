import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWebSocketClient } from "@/lib/ws/connection";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import {
  CoordinatorTaskObservation,
  type CoordinatorFilters,
} from "@/lib/orchestration/coordinator-task-observation";

export function useCoordinatorTasks(workspaceId: string, filters: CoordinatorFilters = {}) {
  const connection = useAppStore((s) => s.connection.status);
  const user = useAppStore((s) => s.auth.user?.id);
  const client = useWebSocketClient();
  const { query, workflowId, repositoryId } = filters;
  const observation = useMemo(
    () => new CoordinatorTaskObservation(workspaceId, { query, workflowId, repositoryId }),
    [workspaceId, query, workflowId, repositoryId, user],
  );
  const snapshot = useSyncExternalStore(
    observation.subscribe,
    observation.getSnapshot,
    observation.getSnapshot,
  );
  useEffect(() => {
    observation.activate();
    void observation.refresh();
    return observation.dispose;
  }, [observation]);
  useEffect(() => {
    if (connection === "connected") observation.scheduleRefresh();
  }, [connection, observation]);
  useEffect(() => {
    if (!client) return;
    const unsubscribe = [
      client.on("task.status_summary.updated", (event) => observation.applySummary(event.payload)),
      client.on("task.created", (event) => observation.lifecycle(event.payload)),
      client.on("task.updated", (event) => observation.lifecycle(event.payload)),
      client.on("task.state_changed", (event) => observation.lifecycle(event.payload)),
      client.on("task.deleted", (event) => observation.lifecycle(event.payload, true)),
    ];
    return () => unsubscribe.forEach((stop) => stop());
  }, [client, observation]);
  useForegroundRefresh(observation.refresh, true, observation);
  return { ...snapshot, refresh: observation.refresh, loadMore: observation.loadMore };
}
