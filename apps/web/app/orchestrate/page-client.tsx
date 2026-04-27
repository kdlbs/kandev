"use client";

import { useCallback, useEffect } from "react";
import { IconRobot, IconCircleDot, IconCurrencyDollar, IconShieldCheck } from "@tabler/icons-react";
import { Card } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import type { DashboardData } from "@/lib/state/slices/orchestrate/types";
import { MetricCard } from "./components/metric-card";
import { ActivityRow } from "./workspace/activity/activity-row";

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

type OrchestratePageClientProps = {
  initialDashboard?: DashboardData | null;
};

const EMPTY_METRICS = {
  agentCount: 0,
  running: 0,
  paused: 0,
  errors: 0,
  tasksInProgress: 0,
  monthSpend: 0,
  pendingApprovals: 0,
  recentActivity: [] as DashboardData["recent_activity"],
};

function extractMetrics(dashboard: DashboardData | null) {
  if (!dashboard) return EMPTY_METRICS;
  return {
    agentCount: dashboard.agent_count,
    running: dashboard.running_count,
    paused: dashboard.paused_count,
    errors: dashboard.error_count,
    tasksInProgress: dashboard.tasks_in_progress,
    monthSpend: dashboard.month_spend_cents,
    pendingApprovals: dashboard.pending_approvals,
    recentActivity: dashboard.recent_activity ?? [],
  };
}

function MetricsGrid({ m }: { m: ReturnType<typeof extractMetrics> }) {
  return (
    <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
      <MetricCard
        icon={IconRobot}
        value={m.agentCount}
        label="Agents Enabled"
        description={`${m.running} running, ${m.paused} paused, ${m.errors} errors`}
      />
      <MetricCard
        icon={IconCircleDot}
        value={m.tasksInProgress}
        label="Tasks In Progress"
        description="Currently running or queued tasks"
      />
      <MetricCard
        icon={IconCurrencyDollar}
        value={formatCents(m.monthSpend)}
        label="Month Spend"
        description="Total API costs this billing period"
      />
      <MetricCard
        icon={IconShieldCheck}
        value={m.pendingApprovals}
        label="Pending Approvals"
        description="Items waiting for your review"
      />
    </div>
  );
}

function RecentActivityCard({
  entries,
}: {
  entries: ReturnType<typeof extractMetrics>["recentActivity"];
}) {
  return (
    <Card>
      <div className="p-4 border-b border-border">
        <h2 className="text-sm font-semibold">Recent Activity</h2>
      </div>
      <div className="divide-y divide-border">
        {entries.length === 0 ? (
          <div className="px-4 py-6 text-center text-sm text-muted-foreground">
            No recent activity. Actions by agents and users will appear here.
          </div>
        ) : (
          entries.map((entry) => <ActivityRow key={entry.id} entry={entry} />)
        )}
      </div>
    </Card>
  );
}

export function OrchestratePageClient({ initialDashboard }: OrchestratePageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const dashboard = useAppStore((s) => s.orchestrate.dashboard);
  const setDashboard = useAppStore((s) => s.setDashboard);

  useEffect(() => {
    if (initialDashboard) {
      setDashboard(initialDashboard);
    }
  }, [initialDashboard, setDashboard]);

  const fetchDashboard = useCallback(async () => {
    if (!workspaceId) return;
    const data = await orchestrateApi.getDashboard(workspaceId);
    setDashboard(data);
  }, [workspaceId, setDashboard]);

  useEffect(() => {
    void fetchDashboard();
  }, [fetchDashboard]);

  const metrics = extractMetrics(dashboard);

  return (
    <div className="space-y-4 p-6">
      <MetricsGrid m={metrics} />
      <div className="grid md:grid-cols-2 gap-4">
        <RecentActivityCard entries={metrics.recentActivity} />
      </div>
    </div>
  );
}
