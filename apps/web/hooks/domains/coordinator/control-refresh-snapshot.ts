import type { PersistenceDiagnostics, SessionCapacityObservation } from "@/lib/types/system";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { ControlConversationLists } from "./control-conversation-lists";
import type { ControlCoordinatorList } from "./use-integrated-control-data";
import type { ControlReadStatus } from "./use-control-resource";

type Collection = {
  error: boolean;
  transient?: boolean;
  receivedAt?: string;
  stale?: boolean;
};

export function retainControlCollection<T extends Collection>(previous: T | undefined, next: T): T {
  if (next.error && next.transient && previous?.receivedAt)
    return {
      ...previous,
      ...next,
      ...retainedItems(previous),
      stale: true,
      receivedAt: previous.receivedAt,
    };
  return { ...next, receivedAt: next.error ? undefined : new Date().toISOString() };
}

function retainedItems<T extends Collection>(previous: T) {
  return {
    ...("items" in previous ? { items: previous.items } : {}),
    ...("total" in previous ? { total: previous.total } : {}),
  };
}

export function retainControlDetails(
  previous:
    | {
        byWorkspace: Record<string, ControlCoordinatorList>;
        conversationsByWorkspace: Record<string, ControlConversationLists>;
      }
    | undefined,
  next: {
    byWorkspace: Record<string, ControlCoordinatorList>;
    conversationsByWorkspace: Record<string, ControlConversationLists>;
  },
) {
  return {
    byWorkspace: Object.fromEntries(
      Object.entries(next.byWorkspace).map(([id, row]) => [
        id,
        retainControlCollection(previous?.byWorkspace[id], row),
      ]),
    ),
    conversationsByWorkspace: Object.fromEntries(
      Object.entries(next.conversationsByWorkspace).map(([id, row]) => {
        const old = previous?.conversationsByWorkspace[id];
        return [
          id,
          {
            independent: retainControlCollection(old?.independent, row.independent),
            tasks: row.tasks && retainControlCollection(old?.tasks, row.tasks),
          },
        ];
      }),
    ),
  };
}

export function confirmedCapacity(observation?: SessionCapacityObservation) {
  const at = parseStrictRfc3339Timestamp(observation?.observed_at ?? "");
  return observation?.known &&
    at !== null &&
    Number(at / BigInt(1_000_000)) <= Date.now() + 5_000 &&
    Number.isSafeInteger(observation.in_use) &&
    Number(observation.in_use) >= 0
    ? observation
    : undefined;
}

type CapacitySnapshot = {
  capacity?: SessionCapacityObservation;
  lastConfirmedCapacity?: SessionCapacityObservation;
  capacityReceivedAt?: string;
};
export function retainCapacity(
  previous: CapacitySnapshot | undefined,
  capacity: SessionCapacityObservation | undefined,
  status: ControlReadStatus,
) {
  const retained = status === "error";
  const allowed = status === "ready" || retained;
  return {
    capacity: retained ? previous?.capacity : capacity,
    capacityStatus: status,
    capacityReceivedAt: retained ? previous?.capacityReceivedAt : new Date().toISOString(),
    lastConfirmedCapacity:
      confirmedCapacity(capacity) ?? (allowed ? previous?.lastConfirmedCapacity : undefined),
  };
}

type PersistenceSnapshot = {
  persistence?: PersistenceDiagnostics;
  persistenceStatus?: ControlReadStatus;
};
/** A transient diagnostics failure keeps the last reading rather than claiming a state. */
export function retainPersistence(
  previous: PersistenceSnapshot | undefined,
  persistence: PersistenceDiagnostics | undefined,
  status: ControlReadStatus,
) {
  const retained = status === "error";
  return {
    persistence: retained ? previous?.persistence : persistence,
    persistenceStatus: status,
  };
}
