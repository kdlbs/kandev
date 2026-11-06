import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { readJourneyWorkspaces } from "@/hooks/journey-metadata-resources";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { listCoordinators, type Coordinator } from "@/lib/api/domains/coordinator-api";
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { useWebSocketClient } from "@/lib/ws/connection";
import {
  createControlRefreshScheduler,
  type ControlRefreshOutcome,
  type ControlRefreshScheduler,
} from "./control-refresh-scheduler";

export type ControlWorkspace = WorkspaceState["items"][number];
export type ControlCoordinatorList = { items: Coordinator[]; error: boolean };
type Snapshot = {
  receivedAt: string;
  identity: string;
  owner: object;
  workspaces: ControlWorkspace[];
  byWorkspace: Record<string, ControlCoordinatorList>;
};

async function readCoordinatorLists(workspaces: ControlWorkspace[], signal: AbortSignal) {
  const rows = await Promise.all(
    workspaces.map(async (workspace): Promise<[string, ControlCoordinatorList]> => {
      try {
        const result = await listCoordinators(workspace.id, {
          cache: "no-store",
          init: { signal },
        });
        return [workspace.id, { items: result.coordinators, error: false }];
      } catch {
        return [workspace.id, { items: [], error: true }];
      }
    }),
  );
  return Object.fromEntries(rows);
}

/**
 * How a failed control read is explained: `unavailable` for a temporarily
 * unavailable store (503 or persistence_unavailable), `disconnected` when the
 * server could not be reached, `denied` for a read the caller may not make
 * (its data is cleared and it is not retried early), `error` otherwise.
 */
export type ControlFailureKind = "unavailable" | "disconnected" | "denied" | "error";

export function controlFailureKind(error: unknown): ControlFailureKind {
  if (error instanceof ApiError) {
    const code =
      error.body && typeof error.body === "object"
        ? (error.body as { code?: unknown }).code
        : undefined;
    if (error.status === 503 || code === "persistence_unavailable") return "unavailable";
    if (error.status === 401 || error.status === 403) return "denied";
  }
  if (error instanceof TypeError) return "disconnected";
  return "error";
}

/** Why the last read failed; the page shows it once, as one banner. */
export type ControlRefreshFailure = {
  kind: ControlFailureKind;
  /** When the next automatic retry runs, if one is scheduled. */
  retryAt?: string;
};

const CONTROL_REFRESH_EVENTS = [
  "coordinator.updated",
  "workspace.updated",
  "workspace.deleted",
] as const;

/**
 * One authorized read lifetime shared by the route and both navigation
 * surfaces. Reads run every 30 seconds, live events are coalesced into one
 * settled read, and a request during a read joins one follow-up read instead
 * of aborting the read in flight. A transient failure keeps the last values.
 */
export function useIntegratedControlData(enabled: boolean, coordinatorEnabled: boolean) {
  const store = useAppStoreApi();
  const identity = useAppStore(stateReadScopeIdentity);
  const owner = useMemo(() => ({}), [enabled, coordinatorEnabled, identity]);
  const ws = useWebSocketClient();
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [loading, setLoading] = useState(false);
  const [failure, setFailure] = useState<ControlRefreshFailure | null>(null);
  const scheduler = useRef<ControlRefreshScheduler | null>(null);
  const read = useCallback(
    async (signal: AbortSignal): Promise<ControlRefreshOutcome> => {
      const current = () =>
        !signal.aborted && stateReadScopeIdentity(store.getState()) === identity;
      setLoading(true);
      try {
        const result = await readJourneyWorkspaces(store, { signal, refresh: true });
        if (!current()) return "ok";
        const workspaces = result.workspaces
          .map(mapWorkspaceItem)
          .filter((item) => hasScope(item.scopes, SCOPE.workspaceRead));
        const byWorkspace = coordinatorEnabled
          ? await readCoordinatorLists(workspaces, signal)
          : {};
        if (!current()) return "ok";
        setSnapshot({
          identity,
          owner,
          workspaces,
          byWorkspace,
          receivedAt: new Date().toISOString(),
        });
        setFailure(null);
        return "ok";
      } catch (error) {
        if (!current()) return "ok";
        const kind = controlFailureKind(error);
        if (kind === "denied") setSnapshot(undefined);
        setFailure((previous) => ({ ...previous, kind }));
        return kind === "denied" ? "ok" : "retry";
      } finally {
        if (current()) setLoading(false);
      }
    },
    [coordinatorEnabled, store, identity, owner],
  );

  useEffect(() => {
    if (!enabled) return;
    const next = createControlRefreshScheduler({
      run: read,
      isVisible: () => document.visibilityState === "visible",
      onRetryScheduled: (at) =>
        setFailure((previous) =>
          previous ? { ...previous, retryAt: at ? new Date(at).toISOString() : undefined } : null,
        ),
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
  useEffect(() => {
    if (!enabled || !ws) return;
    const onEvent = () => scheduler.current?.event();
    const removers = CONTROL_REFRESH_EVENTS.map((event) => ws.on(event, onEvent));
    return () => removers.forEach((remove) => remove());
  }, [enabled, ws]);

  const data =
    enabled && snapshot?.identity === identity && snapshot.owner === owner ? snapshot : undefined;
  const visibleFailure = enabled ? failure : null;
  return { data, loading, error: Boolean(visibleFailure), failure: visibleFailure, refresh };
}
