import { useEffect, useState } from "react";
import { DashboardView } from "@/app/office/agents/[id]/dashboard/dashboard-view";
import { RunsListView } from "@/app/office/agents/[id]/runs/runs-list-view";
import { RunDetailView } from "@/app/office/agents/[id]/runs/[runId]/run-detail-view";
import {
  getAgentSummary,
  getRunDetail,
  listAgentRuns,
  type AgentRunsListPage,
  type AgentSummaryResponse,
  type RunDetail,
} from "@/lib/api/domains/office-extended-api";

const DASHBOARD_DAYS = 14;

type LoadState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "error"; message: string };

export function AgentDashboardRoute({ agentId }: { agentId: string }) {
  const [state, setState] = useState<LoadState<AgentSummaryResponse>>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    getAgentSummary(agentId, DASHBOARD_DAYS, { cache: "no-store" })
      .then((data) => {
        if (!cancelled) setState({ status: "ready", data });
      })
      .catch((error: unknown) => {
        if (!cancelled) setState(toErrorState(error));
      });

    return () => {
      cancelled = true;
    };
  }, [agentId]);

  if (state.status !== "ready") {
    return <AgentRoutePlaceholder state={state} label="agent dashboard" />;
  }

  return <DashboardView agentId={agentId} initial={state.data} days={DASHBOARD_DAYS} />;
}

export function AgentRunsRoute({ agentId }: { agentId: string }) {
  const [state, setState] = useState<LoadState<AgentRunsListPage>>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    listAgentRuns(agentId, { limit: 25 }, { cache: "no-store" })
      .then((data) => {
        if (!cancelled) setState({ status: "ready", data });
      })
      .catch((error: unknown) => {
        if (!cancelled) setState(toErrorState(error));
      });

    return () => {
      cancelled = true;
    };
  }, [agentId]);

  if (state.status !== "ready") {
    return <AgentRoutePlaceholder state={state} label="agent runs" />;
  }

  return <RunsListView agentId={agentId} initial={state.data} />;
}

export function AgentRunDetailRoute({ agentId, runId }: { agentId: string; runId: string }) {
  const [state, setState] = useState<LoadState<{ initial: RunDetail; recent: AgentRunsListPage }>>({
    status: "loading",
  });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    async function loadRunDetail() {
      const [initial, recent] = await Promise.all([
        getRunDetail(agentId, runId, { cache: "no-store" }),
        listAgentRuns(agentId, { limit: 30 }, { cache: "no-store" }),
      ]);
      return { initial, recent };
    }

    loadRunDetail()
      .then((data) => {
        if (!cancelled) setState({ status: "ready", data });
      })
      .catch((error: unknown) => {
        if (!cancelled) setState(toErrorState(error));
      });

    return () => {
      cancelled = true;
    };
  }, [agentId, runId]);

  if (state.status !== "ready") {
    return <AgentRoutePlaceholder state={state} label="agent run" />;
  }

  return (
    <RunDetailView agentId={agentId} initial={state.data.initial} recent={state.data.recent} />
  );
}

function AgentRoutePlaceholder<T>({ state, label }: { state: LoadState<T>; label: string }) {
  if (state.status === "error") {
    return <div className="py-8 text-sm text-destructive">{state.message}</div>;
  }

  return <div className="py-8 text-sm text-muted-foreground">Loading {label}...</div>;
}

function toErrorState(error: unknown): LoadState<never> {
  return {
    status: "error",
    message: error instanceof Error ? error.message : "Failed to load route",
  };
}
