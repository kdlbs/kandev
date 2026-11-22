import type { AppState } from "@/lib/state/store";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { TaskSession } from "@/lib/types/http";
import { NEW_SESSION_SOURCE, type FollowSource } from "./session-selection";
import { findTaskInState } from "./task-lookup";

/** Non-removal follow intent observed while a removal is still being verified. */
export type DeferredFollow = { source: FollowSource; targetSessionId: string | null };

export type FollowOwner = {
  taskId: string;
  previousSessionId: string;
  selectedSessionId: string | null;
  targetSessionId: string | null;
  projectedPrimaryId: string | null | undefined;
  source: FollowSource;
  lifecycleRecipient: boolean;
  scopeIdentity: string;
  navigationRevision: number | undefined;
  pinnedSessionId: string | null;
  workflowFocus: AppState["workflowSessionFocus"];
  deferred?: DeferredFollow;
  /** A resumed owner never re-defers on a second 404, bounding fetch/list disagreement. */
  resumedAfterDisproof?: boolean;
};
export type PendingFollow = FollowOwner & { cancel: () => void; authorized: boolean };
export type RemovalOwner = Pick<FollowOwner, "taskId" | "previousSessionId" | "scopeIdentity">;
export type RemovalTarget = Pick<
  TaskSession,
  "id" | "task_id" | "state" | "started_at" | "updated_at"
>;

export function captureOwner(
  state: AppState,
  taskId: string,
  previousSessionId: string,
  targetSessionId: string | null,
  source: FollowSource,
): FollowOwner {
  return {
    taskId,
    previousSessionId,
    selectedSessionId: previousSessionId,
    targetSessionId,
    source,
    lifecycleRecipient: source === "terminal" || source === NEW_SESSION_SOURCE,
    scopeIdentity: stateReadScopeIdentity(state),
    projectedPrimaryId:
      source === "primary" ? targetSessionId : findTaskInState(state, taskId)?.primarySessionId,
    navigationRevision: state.taskRemoval?.navigationRevision,
    pinnedSessionId: state.tasks.pinnedSessionId,
    workflowFocus: state.workflowSessionFocus,
  };
}

/** A removal observed over an in-flight owner keeps that owner's intent for a disproof. */
export function inheritFollowIntent(owner: FollowOwner, existing: FollowOwner | undefined): void {
  if (existing?.taskId !== owner.taskId) return;
  if (existing.previousSessionId !== owner.previousSessionId) return;
  if (existing.selectedSessionId !== owner.selectedSessionId) return;
  const { source, targetSessionId } = existing;
  owner.deferred = source === "removed" ? existing.deferred : { source, targetSessionId };
  owner.resumedAfterDisproof = existing.resumedAfterDisproof;
  owner.lifecycleRecipient ||= existing.lifecycleRecipient;
}
