"use client";

import { useAppStore } from "@/components/state-provider";

/**
 * Session candidates come from three independently mutable sources: the global
 * active selection (already repointed at the child during creation), the route,
 * and the task's first session. Every candidate must belong to the task this
 * page is rendering, or a retained parent view would show the child's
 * conversation and request its status. Missing ownership stays null until
 * hydration supplies the row; an ineligible candidate falls through to the
 * next one.
 */
export function useRenderedTaskSessionId(
  renderedTaskId: string | null,
  routeSessionId: string | null,
  agentSessionId: string | null,
): string | null {
  const validatedActiveSessionId = useAppStore((state) => {
    const sid = state.tasks.activeSessionId;
    if (!sid || !renderedTaskId) return null;
    return state.taskSessions.items[sid]?.task_id === renderedTaskId ? sid : null;
  });
  const validatedRouteSessionId = useAppStore((state) => {
    if (!routeSessionId || !renderedTaskId) return null;
    return state.taskSessions.items[routeSessionId]?.task_id === renderedTaskId
      ? routeSessionId
      : null;
  });
  const validatedAgentSessionId = useAppStore((state) => {
    if (!agentSessionId || !renderedTaskId) return null;
    return state.taskSessions.items[agentSessionId]?.task_id === renderedTaskId
      ? agentSessionId
      : null;
  });
  return validatedActiveSessionId ?? validatedRouteSessionId ?? validatedAgentSessionId;
}
