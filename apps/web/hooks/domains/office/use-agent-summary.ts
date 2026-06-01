"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient, type QueryKey } from "@tanstack/react-query";
import { getAgentSummary, type AgentSummaryResponse } from "@/lib/api/domains/office-runs-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { BackendMessageType } from "@/lib/types/backend";

// Office WS events that change an agent's dashboard summary (runs,
// activity, cost, and task aggregates). The summary endpoint isn't
// patched in place by the office bridge, so this component-scoped
// subscription invalidates its own query key when these fire — the
// TQ-native replacement for the legacy `useOfficeRefetch` trigger.
const SUMMARY_REFRESH_EVENTS: BackendMessageType[] = [
  "office.run.queued",
  "office.run.processed",
  "office.agent.completed",
  "office.agent.failed",
  "office.task.created",
  "office.task.moved",
  "office.task.status_changed",
  "session.state_changed",
];

export type UseAgentSummaryResult = {
  summary: AgentSummaryResponse;
  isLoading: boolean;
};

/**
 * Agent dashboard summary, read from TanStack Query and seeded from the
 * SSR snapshot via `initialData`. Subscribes to the office WS events that
 * affect the summary and invalidates its own key so the dashboard stays
 * reactive without the Zustand `office.refetchTrigger` mirror.
 */
export function useAgentSummary(
  agentId: string,
  initial: AgentSummaryResponse,
  days?: number,
): UseAgentSummaryResult {
  const qc = useQueryClient();
  const queryKey: QueryKey = ["office", "agents", agentId, "summary", days ?? "default"] as const;

  const { data, isPending } = useQuery({
    queryKey,
    queryFn: () => getAgentSummary(agentId, days),
    initialData: initial,
    staleTime: 30_000,
  });

  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;
    const invalidate = () => void qc.invalidateQueries({ queryKey });
    const unsubs = SUMMARY_REFRESH_EVENTS.map((event) => client.on(event, invalidate));
    return () => {
      for (const unsub of unsubs) unsub();
    };
    // queryKey is derived from agentId + days; spread its inputs instead of
    // the array identity (which is a fresh ref each render).
  }, [qc, agentId, days]); // eslint-disable-line react-hooks/exhaustive-deps

  return { summary: data, isLoading: isPending };
}
