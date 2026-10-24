"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWorkspaceAggregate } from "@/lib/api/domains/office-extended-api";
import { normalizeActivityEntry } from "@/lib/api/domains/office-activity-normalize";

const REFRESH_INTERVAL_MS = 30_000;

export type WorkspaceAggregateLoadState = "loading" | "loaded" | "error";
type InFlightRequest = { promise: Promise<void>; generation: number };

/** Loads the shared workspace overview and refreshes it while a caller is mounted. */
export function useWorkspaceAggregate() {
  const setWorkspaceAggregate = useAppStore((state) => state.setWorkspaceAggregate);
  const [loadState, setLoadState] = useState<WorkspaceAggregateLoadState>("loading");
  const inFlightRef = useRef<InFlightRequest | null>(null);
  const requestGenerationRef = useRef(0);

  const refresh = useCallback((): Promise<void> => {
    if (inFlightRef.current?.generation === requestGenerationRef.current) {
      return inFlightRef.current.promise;
    }

    const requestGeneration = ++requestGenerationRef.current;
    const request = getWorkspaceAggregate({ cache: "no-store" })
      .then((data) => {
        if (requestGeneration !== requestGenerationRef.current) return;
        setWorkspaceAggregate({
          workspaces: data.workspaces ?? [],
          recentActivity: (data.recent_activity ?? []).map(normalizeActivityEntry),
        });
        setLoadState("loaded");
      })
      .catch(() => {
        if (requestGeneration !== requestGenerationRef.current) return;
        setLoadState((current) => (current === "loading" ? "error" : current));
      })
      .finally(() => {
        if (inFlightRef.current?.promise === request) inFlightRef.current = null;
      });

    inFlightRef.current = { promise: request, generation: requestGeneration };
    return request;
  }, [setWorkspaceAggregate]);

  useEffect(() => {
    void refresh();
    const interval = window.setInterval(() => void refresh(), REFRESH_INTERVAL_MS);
    return () => {
      window.clearInterval(interval);
      requestGenerationRef.current++;
    };
  }, [refresh]);

  return { loadState, refresh };
}
