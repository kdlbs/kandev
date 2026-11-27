"use client";

import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getProvenRemovalPredecessor } from "@/lib/state/session-removal-handoff";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";

/** Passive route synchronization cannot consume an independent selection owner. */
export function hasOwnedTaskPageSelection(store: StoreApi<AppState>, taskId: string): boolean {
  const state = store.getState();
  if (state.tasks.activeTaskId !== taskId) return false;
  const sid = state.tasks.activeSessionId;
  return (
    (sid !== null &&
      state.tasks.pinnedSessionId === sid &&
      state.taskSessions.items[sid]?.task_id === taskId) ||
    state.workflowSessionFocus.intent?.taskId === taskId ||
    state.workflowSessionFocus.request?.taskId === taskId ||
    getProvenRemovalPredecessor(store, taskId, sid) !== null
  );
}

export type RenderedTaskSession = {
  /** The owned session the page may render, or null while ownership is unknown. */
  sessionId: string | null;
  /**
   * True while the route names a session that is not owned by the rendered task
   * (missing row or a row owned by another task). Automatic session ensuring is
   * disabled in this state instead of guessing or creating a session.
   */
  routeSessionUnresolved: boolean;
};

/**
 * Session candidates come from three independently mutable sources: the global
 * active selection (already repointed at the child during creation), the route,
 * and the task's first session. Every candidate must belong to the task this
 * page is rendering, or a retained parent view would show the child's
 * conversation and request its status.
 *
 * An explicit route session is authoritative: while its row is missing the page
 * waits instead of substituting the task's first session. A route row owned by
 * another task is ineligible and falls through to an owned fallback. The active
 * selection keeps its existing precedence when it is owned, so a session-tab
 * change does not need a route update.
 */
export function useRenderedTaskSession(
  renderedTaskId: string | null,
  routeSessionId: string | null,
  agentSessionId: string | null,
): RenderedTaskSession {
  const appStore = useAppStoreApi();
  const ownsEmptyRemoval = useAppStore((state) => {
    if (
      !renderedTaskId ||
      state.tasks.activeTaskId !== renderedTaskId ||
      state.tasks.activeSessionId !== null
    )
      return false;
    const proof = state.taskRemoval?.removalHandoff;
    if (proof && proof.taskId === renderedTaskId && proof.targetSessionId === null) {
      return (
        state.taskRemoval.navigationRevision === proof.navigationRevision &&
        stateReadScopeIdentity(state) === proof.scopeIdentity
      );
    }
    return getProvenRemovalPredecessor(appStore, renderedTaskId, null) !== null;
  });
  const validatedActiveSessionId = useAppStore((state) => {
    const sid = state.tasks.activeSessionId;
    if (!sid || !renderedTaskId) return null;
    return state.taskSessions.items[sid]?.task_id === renderedTaskId ? sid : null;
  });
  const routeSessionTaskId = useAppStore((state) =>
    routeSessionId ? (state.taskSessions.items[routeSessionId]?.task_id ?? null) : null,
  );
  const validatedAgentSessionId = useAppStore((state) => {
    if (!agentSessionId || !renderedTaskId) return null;
    return state.taskSessions.items[agentSessionId]?.task_id === renderedTaskId
      ? agentSessionId
      : null;
  });

  if (!renderedTaskId) return { sessionId: null, routeSessionUnresolved: false };
  if (ownsEmptyRemoval) return { sessionId: null, routeSessionUnresolved: true };
  if (validatedActiveSessionId) {
    return {
      sessionId: validatedActiveSessionId,
      routeSessionUnresolved: routeSessionId !== null && routeSessionTaskId !== renderedTaskId,
    };
  }
  if (routeSessionId) {
    if (routeSessionTaskId === renderedTaskId) {
      return { sessionId: routeSessionId, routeSessionUnresolved: false };
    }
    if (routeSessionTaskId === null) {
      return { sessionId: null, routeSessionUnresolved: true };
    }
    return { sessionId: validatedAgentSessionId, routeSessionUnresolved: true };
  }
  return { sessionId: validatedAgentSessionId, routeSessionUnresolved: false };
}
