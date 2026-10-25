"use client";

import { useEffect, useState } from "react";
import { useWebSocketClient } from "@/lib/ws/connection";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { useCoordinatorList } from "./use-coordinator-list";

export type UseCoordinatorSidebarEntriesResult = {
  /** The workspace's coordinators, in list order. Undefined before the first successful read. */
  coordinators: Coordinator[] | undefined;
  /**
   * Open-proposal badge count by coordinator id (system-design/needs-you.md
   * #routes-and-sidebar). Seeded by `listCoordinators`'s `open_proposals`
   * field and replaced, per coordinator, by each `coordinator.updated`
   * payload — without a reload (AC-COORDINATOR-NEEDS-YOU-006.2).
   */
  badgeByCoordinatorId: Map<string, number>;
};

/**
 * Backs the coordinator sidebar entries (desktop `AppSidebarFixedNav` and
 * `MobileRequiredRows`): the coordinator list itself, plus a small map of
 * open-proposal badge counts kept live over `coordinator.updated`.
 */
export function useCoordinatorSidebarEntries(
  workspaceId: string | null,
): UseCoordinatorSidebarEntriesResult {
  const { coordinators } = useCoordinatorList(workspaceId);
  const wsClient = useWebSocketClient();
  const [overrides, setOverrides] = useState<Map<string, number>>(new Map());

  useEffect(() => {
    setOverrides(new Map());
  }, [workspaceId]);

  useEffect(() => {
    if (!wsClient || !workspaceId) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId) return;
      setOverrides((prev) => {
        const next = new Map(prev);
        next.set(payload.coordinator_id, payload.open_proposals);
        return next;
      });
    });
  }, [wsClient, workspaceId]);

  const badgeByCoordinatorId = new Map<string, number>();
  for (const coordinator of coordinators ?? []) {
    badgeByCoordinatorId.set(
      coordinator.id,
      overrides.get(coordinator.id) ?? coordinator.open_proposals ?? 0,
    );
  }

  return { coordinators, badgeByCoordinatorId };
}
