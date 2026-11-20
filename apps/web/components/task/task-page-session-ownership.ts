"use client";

import { useAppStore } from "@/components/state-provider";

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
