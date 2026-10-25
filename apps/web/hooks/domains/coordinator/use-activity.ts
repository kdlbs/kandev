"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWebSocketClient } from "@/lib/ws/connection";
import type { ActivityClass, ActivityItem } from "@/lib/api/domains/coordinator-activity-api";
import {
  ActivityController,
  INITIAL_ACTIVITY_SNAPSHOT,
  type ActivitySnapshot,
} from "./activity-controller";
import { useActivityMembers } from "./use-activity-members";

type Held = { controller: ActivityController; key: string };

const IDLE_STORE = {
  subscribe: () => () => undefined,
  getSnapshot: (): ActivitySnapshot => INITIAL_ACTIVITY_SNAPSHOT,
};

export type UseActivityParams = {
  workspaceId: string;
  coordinatorId: string;
  activityClass: ActivityClass | undefined;
};

/**
 * The What it did list for one (coordinator, class filter): loading, paging,
 * re-reads on `coordinator.updated` and reconnect, and Undo with its messages
 * (docs/specs/coordinator/system-design/activity-log.md#what-it-did-ui).
 */
export function useActivity({ workspaceId, coordinatorId, activityClass }: UseActivityParams) {
  const members = useActivityMembers(workspaceId);
  const noteArrival = useRef(members.noteArrival);
  useEffect(() => {
    noteArrival.current = members.noteArrival;
  }, [members.noteArrival]);

  const key = `${workspaceId}|${coordinatorId}|${activityClass ?? ""}`;
  const [held, setHeld] = useState<Held | null>(null);

  useEffect(() => {
    const controller = new ActivityController({
      workspaceId,
      coordinatorId,
      activityClass,
      onArrival: (rows: ActivityItem[]) => noteArrival.current(rows),
    });
    setHeld({ controller, key });
    controller.start();
    return () => controller.dispose();
  }, [workspaceId, coordinatorId, activityClass, key]);

  const controller = held && held.key === key ? held.controller : null;
  const store = controller ?? IDLE_STORE;
  const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient || !controller) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      controller.refresh();
    });
  }, [wsClient, controller, workspaceId, coordinatorId]);

  const connectionStatus = useAppStore((s) => s.connection.status);
  const previousConnection = useRef(connectionStatus);
  useEffect(() => {
    const previous = previousConnection.current;
    previousConnection.current = connectionStatus;
    if (connectionStatus === "connected" && previous !== "connected") controller?.refresh();
  }, [connectionStatus, controller]);

  const loadMore = useCallback(() => controller?.loadMore(), [controller]);
  const retry = useCallback(() => controller?.retry(), [controller]);
  const undo = useCallback((row: ActivityItem) => void controller?.undo(row), [controller]);

  return { snapshot, loadMore, retry, undo, resolvePerson: members.resolve };
}
