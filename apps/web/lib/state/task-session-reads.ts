import type { AppState } from "@/lib/state/store";
import type { Task, TaskSessionsResponse } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import { SharedResourceReads, storeReadScopeIdentity } from "./shared-resource-reads";
import { listTaskSessions } from "@/lib/api";
import { raceAbort, abortError } from "./task-navigation-attempt";

export type TaskSessionReads = SharedResourceReads<TaskSessionsResponse>;

type MembershipProof = { taskId: string; apply: (response: TaskSessionsResponse) => void };
type TaskSessionReadOwner = {
  identity: string;
  reads: TaskSessionReads;
  proofs: Map<string, MembershipProof>;
};
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
      proofs: new Map(),
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

/** Removal proof consumers observe only admitted, current-scope membership responses. */
export function registerTaskSessionMembershipProof(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
  apply: MembershipProof["apply"],
) {
  getTaskSessionReads(store);
  owners.get(store)!.proofs.set(sessionId, { taskId, apply });
}

export function forgetTaskSessionMembershipProof(store: StoreApi<AppState>, sessionId: string) {
  owners.get(store)?.proofs.delete(sessionId);
}

export function publishTaskSessionMembership(
  store: StoreApi<AppState>,
  reads: TaskSessionReads,
  taskId: string,
  generation: number,
  response: TaskSessionsResponse,
) {
  const owner = owners.get(store);
  if (owner?.reads !== reads || !reads.isCurrent(taskId, generation)) return;
  for (const proof of owner.proofs.values()) {
    if (proof.taskId === taskId) proof.apply(response);
  }
}

export async function readTaskNavigationMembership(
  reads: TaskSessionReads,
  taskId: string,
  signal: AbortSignal,
  scopeIsCurrent: () => boolean,
) {
  while (!signal.aborted && scopeIsCurrent()) {
    const generation = reads.generation(taskId);
    try {
      const allSessionsResponse = await raceAbort(
        reads.read(taskId, (readSignal) =>
          listTaskSessions(taskId, { cache: "no-store", init: { signal: readSignal } }),
        ),
        signal,
      );
      if (reads.isCurrent(taskId, generation))
        return {
          allSessionsResponse,
          membershipGeneration: generation,
          sessionListUnavailable: false,
        };
    } catch (error) {
      if (signal.aborted || !scopeIsCurrent() || !reads.isCurrent(taskId)) throw abortError();
      if (error instanceof Error && error.name === "AbortError") {
        if (reads.generation(taskId) === generation) throw error;
        continue;
      }
      return {
        allSessionsResponse: { sessions: [], total: 0 } as TaskSessionsResponse,
        membershipGeneration: generation,
        sessionListUnavailable: true,
      };
    }
  }
  throw abortError();
}

/** Conflicting DTO and membership observations require a later membership read. */
export async function reconcileTaskNavigationMembership(
  task: Task,
  initial: Awaited<ReturnType<typeof readTaskNavigationMembership>>,
  reads: TaskSessionReads,
  signal: AbortSignal,
  options: { scopeIsCurrent: () => boolean; requestedSessionId?: string },
) {
  const { scopeIsCurrent, requestedSessionId } = options;
  if (signal.aborted || !scopeIsCurrent()) throw abortError();
  let membership = initial;
  while (!reads.isCurrent(task.id, membership.membershipGeneration)) {
    membership = await readTaskNavigationMembership(reads, task.id, signal, scopeIsCurrent);
  }
  const sessions = membership.allSessionsResponse.sessions;
  const dtoPrimary = sessions.find(
    (session) => session.task_id === task.id && session.id === task.primary_session_id,
  );
  if (
    !membership.sessionListUnavailable &&
    !sessions.some((session) => session.task_id === task.id && session.id === requestedSessionId) &&
    task.primary_session_id &&
    (!dtoPrimary ||
      dtoPrimary.is_primary === false ||
      sessions.some(
        (session) =>
          session.task_id === task.id &&
          session.is_primary === true &&
          session.id !== dtoPrimary.id,
      ))
  ) {
    reads.invalidate(task.id);
    membership = await readTaskNavigationMembership(reads, task.id, signal, scopeIsCurrent);
  }
  while (!reads.isCurrent(task.id, membership.membershipGeneration)) {
    if (signal.aborted || !scopeIsCurrent()) throw abortError();
    membership = await readTaskNavigationMembership(reads, task.id, signal, scopeIsCurrent);
  }
  return membership;
}
