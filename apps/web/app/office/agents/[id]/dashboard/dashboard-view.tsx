"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { qk } from "@/lib/query/keys";
import { officeAgentSummaryQueryOptions } from "@/lib/query/query-options/office";
import type { AgentSummaryResponse } from "@/lib/api/domains/office-extended-api";
import { LatestRunCard } from "./components/latest-run-card";
import { RunActivityChart } from "./components/run-activity-chart";
import { TasksByPriorityChart } from "./components/tasks-by-priority-chart";
import { TasksByStatusChart } from "./components/tasks-by-status-chart";
import { SuccessRateChart } from "./components/success-rate-chart";
import { RecentTasks } from "./components/recent-tasks";
import { CostsSection } from "./components/costs-section";

type Props = {
  agentId: string;
  initial: AgentSummaryResponse;
  /** When set, the view refetches on this many days. Server defaults to 14. */
  days?: number;
};

/**
 * Client-side shell for the agent dashboard. Holds the SSR snapshot
 * in `useState` and refetches via WebSocket-driven triggers (the
 * `agents` and `tasks` channels both impact the dashboard) — matching
 * the project's reactive-only convention.
 *
 * The chart components are pure route-safe presentational pieces; this
 * shell exists so a future "Refresh" / "Date range" UI has somewhere
 * to live without lifting state up into the parent route loader.
 */
export function DashboardView({ agentId, initial, days }: Props) {
  const queryClient = useQueryClient();
  const summaryQuery = useQuery(officeAgentSummaryQueryOptions(agentId, days));

  useEffect(() => {
    queryClient.setQueryData(qk.office.agentSummary(agentId, days), initial);
  }, [agentId, days, initial, queryClient]);

  // The dashboard derives from runs, activity_log, cost_events, and
  // tasks — every WS event in those domains can change the values, so
  // we subscribe to both `agents` and `tasks` triggers.
  useOfficeRefetch("agents", () => void summaryQuery.refetch());
  useOfficeRefetch("tasks", () => void summaryQuery.refetch());

  const summary = summaryQuery.data ?? initial;
  return (
    <div className="space-y-6" data-testid="agent-dashboard-view">
      <LatestRunCard run={summary.latest_run} agentId={agentId} />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4" data-testid="dashboard-charts">
        <RunActivityChart days={summary.run_activity} />
        <TasksByPriorityChart days={summary.tasks_by_priority} />
        <TasksByStatusChart days={summary.tasks_by_status} />
        <SuccessRateChart days={summary.success_rate} />
      </div>

      <RecentTasks tasks={summary.recent_tasks} />

      <CostsSection
        agentId={agentId}
        aggregate={summary.cost_aggregate}
        recent={summary.recent_run_costs}
      />
    </div>
  );
}
