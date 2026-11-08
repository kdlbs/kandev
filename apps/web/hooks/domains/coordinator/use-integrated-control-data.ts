import { fetchSessionCapacityObservation } from "@/lib/api/domains/session-capacity-api";
import { fetchPersistenceDiagnostics } from "@/lib/api/domains/system-api";
import type { PersistenceDiagnostics, SessionCapacityObservation } from "@/lib/types/system";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
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
  readControlConversationLists,
  type ControlConversationLists,
} from "./control-conversation-lists";
import {
  controlFailureKind,
  controlReadFailure,
  type ControlFailureKind,
  type ControlReadStatus,
} from "./use-control-resource";
import {
  createControlRefreshScheduler,
  type ControlRefreshOutcome,
  type ControlRefreshScheduler,
} from "./control-refresh-scheduler";
import {
  retainCapacity,
  retainControlDetails,
  retainPersistence,
} from "./control-refresh-snapshot";

export type ControlWorkspace = WorkspaceState["items"][number];
export type ControlCoordinatorList = {
  items: Coordinator[];
  error: boolean;
  transient?: boolean;
  receivedAt?: string;
  stale?: boolean;
};
type Snapshot = {
  receivedAt?: string;
  capacity?: SessionCapacityObservation;
  lastConfirmedCapacity?: SessionCapacityObservation;
  capacityStatus?: ControlReadStatus;
  capacityReceivedAt?: string;
  persistence?: PersistenceDiagnostics;
  persistenceStatus?: ControlReadStatus;
  identity: string;
  owner: object;
  workspaces: ControlWorkspace[];
  byWorkspace: Record<string, ControlCoordinatorList>;
  conversationsByWorkspace: Record<string, ControlConversationLists>;
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
      } catch (error) {
        return [
          workspace.id,
          { items: [], error: true, transient: controlReadFailure(error) === "error" },
        ];
      }
    }),
  );
  return Object.fromEntries(rows);
}

/** Why the last snapshot read failed; the page shows it once, as one banner. */
export type ControlRefreshFailure = {
  kind: ControlFailureKind;
  /** When the next automatic retry runs, if one is scheduled. */
  retryAt?: string;
};

/**
 * One authorized read lifetime shared by the route and both navigation
 * surfaces. Reads run every 30 seconds, live events are coalesced into one
 * settled read, and a request during a read joins one follow-up read instead
 * of aborting the read in flight.
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
  const committedOwner = useRef<object | null>(null);
  useLayoutEffect(() => {
    committedOwner.current = owner;
    return () => {
      committedOwner.current = null;
      scheduler.current?.dispose();
    };
  }, [owner]);
  const read = useCallback(
    async (signal: AbortSignal): Promise<ControlRefreshOutcome> => {
      const current = () =>
        !signal.aborted &&
        committedOwner.current === owner &&
        stateReadScopeIdentity(store.getState()) === identity;
      if (!enabled || !current()) return "ok";
      setLoading(true);
      try {
        const result = await readJourneyWorkspaces(store, { signal, refresh: true });
        if (!current()) return "ok";
        const workspaces = result.workspaces
          .map(mapWorkspaceItem)
          .filter((item) => hasScope(item.scopes, SCOPE.workspaceRead));
        const details = await readControlDetails(workspaces, coordinatorEnabled, signal);
        if (!current()) return "ok";
        setSnapshot((previous) => ({
          identity,
          owner,
          workspaces,
          ...details,
          ...retainControlDetails(previous?.owner === owner ? previous : undefined, details),
          ...retainCapacity(
            previous?.owner === owner ? previous : undefined,
            details.capacity,
            details.capacityStatus,
          ),
          ...retainPersistence(
            previous?.owner === owner ? previous : undefined,
            details.persistence,
            details.persistenceStatus,
          ),
          receivedAt: new Date().toISOString(),
        }));
        setFailure(null);
        return "ok";
      } catch (error) {
        if (!current()) return "ok";
        const kind = controlFailureKind(error);
        if (kind === "denied") setSnapshot(undefined);
        setFailure((previous) => ({ ...previous, kind }));
        return kind === "denied" || kind === "unsupported" ? "ok" : "retry";
      } finally {
        if (current()) setLoading(false);
      }
    },
    [enabled, coordinatorEnabled, store, identity, owner],
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

const CONTROL_REFRESH_EVENTS = [
  "coordinator.updated",
  "workspace.updated",
  "workspace.deleted",
  "task.created",
  "task.updated",
  "task.deleted",
  "session.state_changed",
] as const;

async function readControlDetails(
  workspaces: ControlWorkspace[],
  coordinatorEnabled: boolean,
  signal: AbortSignal,
) {
  const [byWorkspace, conversationsByWorkspace, capacity, persistence] = await Promise.all([
    coordinatorEnabled ? readCoordinatorLists(workspaces, signal) : Promise.resolve({}),
    readControlConversationLists(workspaces, signal),
    fetchSessionCapacityObservation({ init: { signal } }).then(
      (value) => ({ value, status: "ready" as const }),
      (error: unknown) => ({ value: undefined, status: controlReadFailure(error) }),
    ),
    fetchPersistenceDiagnostics({ cache: "no-store", init: { signal } }).then(
      (value) => ({ value, status: "ready" as const }),
      (error: unknown) => ({ value: undefined, status: controlReadFailure(error) }),
    ),
  ]);
  return {
    byWorkspace,
    conversationsByWorkspace,
    capacity: capacity.value,
    capacityStatus: capacity.status,
    persistence: persistence.value,
    persistenceStatus: persistence.status,
  };
}
