"use client";

import { useAgentSummary } from "@/hooks/domains/office/use-agent-summary";
import type { AgentSummaryResponse } from "@/lib/api/domains/office-runs-api";
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
 * Client-side shell for the agent dashboard. The summary is read from
 * TanStack Query (seeded from the SSR snapshot via `initialData`) and
 * stays reactive via `useAgentSummary`, which subscribes to the office
 * WS events that affect the dashboard and invalidates its own key —
 * matching the project's reactive-only convention.
 *
 * The chart components are pure SSR-safe presentational pieces; this
 * shell exists so a future "Refresh" / "Date range" UI has somewhere
 * to live without lifting state up into the parent page (which is a
 * Server Component and therefore can't hold state).
 */
export function DashboardView({ agentId, initial, days }: Props) {
  const { summary } = useAgentSummary(agentId, initial, days);

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
