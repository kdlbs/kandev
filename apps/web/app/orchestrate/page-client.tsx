"use client";

import { useCallback, useEffect } from "react";
import {
  IconRobot,
  IconCircleDot,
  IconCurrencyDollar,
  IconShieldCheck,
} from "@tabler/icons-react";
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

  const agentCount = dashboard?.agent_count ?? 0;
  const running = dashboard?.running_count ?? 0;
  const paused = dashboard?.paused_count ?? 0;
  const errors = dashboard?.error_count ?? 0;
  const monthSpend = dashboard?.month_spend_cents ?? 0;
  const pendingApprovals = dashboard?.pending_approvals ?? 0;
  const recentActivity = dashboard?.recent_activity ?? [];

  return (
    <div className="space-y-6 p-6">
      {/* Metric cards */}
      <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
        <MetricCard
          icon={IconRobot}
          value={agentCount}
          label="Agents Enabled"
          description={`${running} running, ${paused} paused, ${errors} errors`}
        />
        <MetricCard
          icon={IconCircleDot}
          value={dashboard?.tasks_in_progress ?? 0}
          label="Tasks In Progress"
        />
        <MetricCard
          icon={IconCurrencyDollar}
          value={formatCents(monthSpend)}
          label="Month Spend"
        />
        <MetricCard
          icon={IconShieldCheck}
          value={pendingApprovals}
          label="Pending Approvals"
        />
      </div>

      {/* Recent activity + recent tasks */}
      <div className="grid md:grid-cols-2 gap-4">
        <Card>
          <div className="p-4 border-b border-border">
            <h2 className="text-sm font-semibold">Recent Activity</h2>
          </div>
          <div className="divide-y divide-border">
            {recentActivity.length === 0 ? (
              <div className="px-4 py-6 text-center text-sm text-muted-foreground">
                No recent activity
              </div>
            ) : (
              recentActivity.map((entry) => (
                <ActivityRow key={entry.id} entry={entry} />
              ))
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}
