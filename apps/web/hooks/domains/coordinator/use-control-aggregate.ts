import { useCallback, useEffect, useRef, useState } from "react";
import {
  createControlRefreshScheduler,
  type ControlRefreshOutcome,
  type ControlRefreshScheduler,
} from "./control-refresh-scheduler";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import {
  getWorkspaceAggregate,
  type WorkspaceAggregateEntry,
} from "@/lib/api/domains/office-extended-api";
import { normalizeActivityEntry } from "@/lib/api/domains/office-activity-normalize";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { ActivityEntry } from "@/lib/state/slices/office/types";

export type ControlAggregate = {
  workspaces: WorkspaceAggregateEntry[];
  recentActivity: ActivityEntry[];
};
export type ControlAggregateRead = {
  data?: ControlAggregate;
  loading: boolean;
  error: boolean;
  refresh: () => Promise<void>;
};

type Snapshot = { identity: string; data: ControlAggregate };

/**
 * Reads the read-only multi-workspace aggregate for the integrated control
 * dashboard. One request covers every Office workspace the caller can reach;
 * a snapshot read under another identity is never shown. It re-reads every
 * 30 seconds; a request during a read joins one follow-up read instead of
 * aborting it. A failed refresh keeps the last good snapshot, reports the
 * error beside it and retries with backoff.
 */
export function useControlAggregate(enabled: boolean): ControlAggregateRead {
  const store = useAppStoreApi();
  const identity = useAppStore(stateReadScopeIdentity);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const scheduler = useRef<ControlRefreshScheduler | null>(null);

  const read = useCallback(
    async (signal: AbortSignal): Promise<ControlRefreshOutcome> => {
      const current = () =>
        !signal.aborted && stateReadScopeIdentity(store.getState()) === identity;
      setLoading(true);
      try {
        const wire = await getWorkspaceAggregate({ cache: "no-store", init: { signal } });
        if (!current()) return "ok";
        setSnapshot({
          identity,
          data: {
            workspaces: wire.workspaces ?? [],
            recentActivity: (wire.recent_activity ?? []).map(normalizeActivityEntry),
          },
        });
        setError(false);
        return "ok";
      } catch {
        if (current()) setError(true);
        return "retry";
      } finally {
        if (current()) setLoading(false);
      }
    },
    [store, identity],
  );

  useEffect(() => {
    if (!enabled) return;
    const next = createControlRefreshScheduler({
      run: read,
      isVisible: () => document.visibilityState === "visible",
    });
    scheduler.current = next;
    void next.request();
    return () => {
      next.dispose();
      if (scheduler.current === next) scheduler.current = null;
    };
  }, [enabled, read]);
  const refresh = useCallback(() => scheduler.current?.request() ?? Promise.resolve(), []);
  useForegroundRefresh(refresh, enabled, identity);

  const data = enabled && snapshot?.identity === identity ? snapshot.data : undefined;
  return { data, loading, error: enabled && error, refresh };
}
