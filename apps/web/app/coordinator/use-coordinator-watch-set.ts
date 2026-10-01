"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getCoordinatorSettings,
  type CoordinatorSettings,
} from "@/lib/api/domains/coordinator-api";
import type { WatchSet } from "@/lib/coordinator/watch-filter";
import { useWebSocketClient } from "@/lib/ws/connection";
import type { CoordinatorInputEntry } from "./use-coordinator-inputs";

export type UseCoordinatorWatchSetResult = {
  input: CoordinatorInputEntry<WatchSet>;
  retry: () => void;
};

/** Maps the wire (snake_case) settings read to the client filter's watch set. */
export function watchSetFromSettings(settings: CoordinatorSettings): WatchSet {
  const projects = settings.projects;
  return {
    scope: settings.watches.scope,
    workflowIds: settings.watches.workflow_ids,
    ...(projects && {
      projects: {
        scope: "selected" as const,
        repositoryIds: projects.repository_ids ?? null,
        includeNoRepository: projects.include_no_repository,
      },
    }),
  };
}

const EMPTY: CoordinatorInputEntry<WatchSet> = {
  value: undefined,
  loadedAt: undefined,
  error: false,
};

/**
 * The coordinator's effective watch set (`GET settings`). It reads on mount,
 * on retry and on each `coordinator.updated` for this coordinator; only the
 * latest read is applied, a failed re-read keeps the last value, and the kept
 * value is discarded when the viewed coordinator changes. Inert while
 * `enabled` is false.
 */
export function useCoordinatorWatchSet(
  workspaceId: string | null,
  coordinatorId: string | null,
  enabled: boolean,
): UseCoordinatorWatchSetResult {
  const [input, setInput] = useState(EMPTY);
  const sequenceRef = useRef(0);
  const wsClient = useWebSocketClient();
  const active = enabled && workspaceId !== null && coordinatorId !== null;

  const read = useCallback(() => {
    if (!workspaceId || !coordinatorId) return;
    const sequence = ++sequenceRef.current;
    getCoordinatorSettings(workspaceId, coordinatorId)
      .then((settings) => {
        if (sequence !== sequenceRef.current) return;
        setInput({
          value: watchSetFromSettings(settings),
          loadedAt: Date.now(),
          error: false,
        });
      })
      .catch(() => {
        if (sequence !== sequenceRef.current) return;
        setInput((prev) => ({ ...prev, error: true }));
      });
  }, [workspaceId, coordinatorId]);

  useEffect(() => {
    setInput(EMPTY);
    if (!active) return;
    read();
    return () => {
      sequenceRef.current += 1;
    };
  }, [active, read]);

  useEffect(() => {
    if (!wsClient || !active) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      read();
    });
  }, [wsClient, active, workspaceId, coordinatorId, read]);

  // A set changing membership changes which repositories a selected Projects
  // scope resolves to; the server resolves, so re-read. A deletion is covered by
  // the coordinator.updated the server publishes after it tidies the entries.
  useEffect(() => {
    if (!wsClient || !active) return;
    const off = (
      ["repository_set.created", "repository_set.updated", "repository_set.deleted"] as const
    ).map((event) => wsClient.on(event, () => read()));
    return () => off.forEach((unsubscribe) => unsubscribe());
  }, [wsClient, active, read]);

  return { input, retry: read };
}
