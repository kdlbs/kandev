import type { AppState } from "@/lib/state/store";
import type { TaskSessionsResponse } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import { SharedResourceReads, storeReadScopeIdentity } from "./shared-resource-reads";
import { listTaskSessions } from "@/lib/api";
import { raceAbort, abortError } from "./task-navigation-attempt";

export type TaskSessionReads = SharedResourceReads<TaskSessionsResponse>;

type TaskSessionReadOwner = { identity: string; reads: TaskSessionReads };
const owners = new WeakMap<StoreApi<AppState>, TaskSessionReadOwner>();

export function createTaskSessionReads(scopeIsCurrent?: () => boolean): TaskSessionReads {
  return new SharedResourceReads<TaskSessionsResponse>(scopeIsCurrent);
}

export function getTaskSessionReads(store: StoreApi<AppState>): TaskSessionReads {
  const identity = storeReadScopeIdentity(store);
  let owner = owners.get(store);
  if (!owner || owner.identity !== identity) {
    owner?.reads.dispose();
    const nextOwner: TaskSessionReadOwner = {
      identity,
      reads: new SharedResourceReads<TaskSessionsResponse>(
        () => owners.get(store) === nextOwner && storeReadScopeIdentity(store) === identity,
      ),
    };
    owner = nextOwner;
    owners.set(store, owner);
  }
  return owner.reads;
}

/** Confirmed removals invalidate old membership snapshots without creating an owner. */
export function invalidateTaskSessionMembership(store: StoreApi<AppState>, taskId: string): void {
  owners.get(store)?.reads.invalidate(taskId);
}

export async function readCurrentMembership(
  reads: TaskSessionReads,
  taskId: string,
  signal: AbortSignal,
  scopeIsCurrent: () => boolean,
) {
  while (!signal.aborted && scopeIsCurrent()) {
    const generation = reads.generation(taskId);
    try {
      return await raceAbort(
        reads.read(taskId, (readSignal) =>
          listTaskSessions(taskId, { cache: "no-store", init: { signal: readSignal } }),
        ),
        signal,
      );
    } catch (error) {
      if (
        signal.aborted ||
        !scopeIsCurrent() ||
        !(error instanceof Error && error.name === "AbortError") ||
        !reads.isCurrent(taskId) ||
        reads.generation(taskId) === generation
      )
        throw error;
    }
  }
  throw abortError();
}
