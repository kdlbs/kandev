import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { DashboardView } from "@/app/office/agents/[id]/dashboard/dashboard-view";
import { RunsListView } from "@/app/office/agents/[id]/runs/runs-list-view";
import { RunDetailView } from "@/app/office/agents/[id]/runs/[runId]/run-detail-view";

import {
  officeAgentRunsInfiniteQueryOptions,
  officeAgentSummaryQueryOptions,
  officeRunDetailQueryOptions,
} from "@/lib/query/query-options/office";
import type { LoadState } from "@/lib/routing/client-route-helpers";

const DASHBOARD_DAYS = 14;

export function AgentDashboardRoute({ agentId }: { agentId: string }) {
  const query = useQuery(officeAgentSummaryQueryOptions(agentId, DASHBOARD_DAYS));
  const state = queryState(query.data, query.error);

  if (state.status !== "ready") {
    return <AgentRoutePlaceholder state={state} label="agent dashboard" />;
  }

  return <DashboardView agentId={agentId} initial={state.data} days={DASHBOARD_DAYS} />;
}

export function AgentRunsRoute({ agentId }: { agentId: string }) {
  const query = useInfiniteQuery(officeAgentRunsInfiniteQueryOptions(agentId, { limit: 25 }));
  const state = queryState(query.data?.pages[0], query.error);

  if (state.status !== "ready") {
    return <AgentRoutePlaceholder state={state} label="agent runs" />;
  }

  return <RunsListView agentId={agentId} initial={state.data} />;
}

export function AgentRunDetailRoute({ agentId, runId }: { agentId: string; runId: string }) {
  const detailQuery = useQuery(officeRunDetailQueryOptions(agentId, runId));
  const recentQuery = useInfiniteQuery(officeAgentRunsInfiniteQueryOptions(agentId, { limit: 30 }));
  const initial = detailQuery.data;
  const recent = recentQuery.data?.pages[0];
  const state = queryState(
    initial && recent ? { initial, recent } : undefined,
    detailQuery.error ?? recentQuery.error,
  );

  if (state.status !== "ready") {
    return <AgentRoutePlaceholder state={state} label="agent run" />;
  }

  return (
    <RunDetailView agentId={agentId} initial={state.data.initial} recent={state.data.recent} />
  );
}

function queryState<T>(data: T | undefined, error: unknown): LoadState<T> {
  if (data !== undefined) return { status: "ready", data };
  if (error) {
    return {
      status: "error",
      message: error instanceof Error ? error.message : "Failed to load route",
    };
  }
  return { status: "loading" };
}

function AgentRoutePlaceholder<T>({ state, label }: { state: LoadState<T>; label: string }) {
  if (state.status === "error") {
    return <div className="py-8 text-sm text-destructive">{state.message}</div>;
  }

  return <div className="py-8 text-sm text-muted-foreground">Loading {label}...</div>;
}
