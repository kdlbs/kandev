"use client";

import { useCallback, useEffect } from "react";
import Link from "next/link";
import {
  IconRobot,
  IconCircleDot,
  IconCurrencyDollar,
  IconShieldCheck,
  IconChartBar,
} from "@tabler/icons-react";
import { Card } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import * as officeApi from "@/lib/api/domains/office-api";
import { normalizeActivityEntry } from "@/lib/api/domains/office-activity-normalize";
import { StatusIcon } from "./tasks/status-icon";
import type { DashboardData, AgentProfile, RecentTask } from "@/lib/state/slices/office/types";
import { MetricCard } from "./components/metric-card";
import { ActivityRow } from "./workspace/activity/activity-row";
import { RunActivityChart, SuccessRateChart } from "./components/dashboard-charts";
import { AgentCardsPanel } from "./components/agent-cards-panel";
import { ProviderHealthCard } from "./components/routing/provider-health-card";
import { timeAgo } from "./components/shared/time-ago";

import { UtilizationBars } from "./components/utilization-bars";
import { formatDollars } from "@/lib/utils";

// formatMonthSpend renders the subcents value from /office dashboard
// as USD. The shared formatDollars helper owns the unit boundary; this
// is a local alias for readability.
function formatMonthSpend(subcents: number): string {
  return formatDollars(subcents);
}

type OfficePageClientProps = {
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
  taskBreakdown: { open: 0, in_progress: 0, blocked: 0, done: 0 },
};

function extractMetrics(dashboard: DashboardData | null) {
  if (!dashboard) return EMPTY_METRICS;
  return {
    agentCount: dashboard.agent_count,
    running: dashboard.running_count,
    paused: dashboard.paused_count,
    errors: dashboard.error_count,
    tasksInProgress: dashboard.tasks_in_progress,
    monthSpend: dashboard.month_spend_subcents,
    pendingApprovals: dashboard.pending_approvals,
    recentActivity: (dashboard.recent_activity ?? []).map(normalizeActivityEntry),
    taskBreakdown: dashboard.task_breakdown ?? { open: 0, in_progress: 0, blocked: 0, done: 0 },
  };
}

function MetricsGrid({ m }: { m: ReturnType<typeof extractMetrics> }) {
  const tb = m.taskBreakdown;
  return (
    <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
      <Link href="/office/agents" className="cursor-pointer">
        <MetricCard
          icon={IconRobot}
          value={m.agentCount}
          label="Agents Enabled"
          description={`${m.running} running, ${m.paused} paused, ${m.errors} errors`}
        />
      </Link>
      <Link href="/office/tasks" className="cursor-pointer">
        <MetricCard
          icon={IconCircleDot}
          value={m.tasksInProgress}
          label="Tasks In Progress"
          description={`${tb.open} open, ${tb.blocked} blocked`}
        />
      </Link>
      <Link href="/office/workspace/costs" className="cursor-pointer">
        <MetricCard
          icon={IconCurrencyDollar}
          value={formatMonthSpend(m.monthSpend)}
          label="Month Spend"
          description="Total API costs this billing period"
        />
      </Link>
      <Link href="/office/inbox" className="cursor-pointer">
        <MetricCard
          icon={IconShieldCheck}
          value={m.pendingApprovals}
          label="Pending Approvals"
          description="Items waiting for your review"
        />
      </Link>
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

function resolveAgentInitials(agentId: string, agents: AgentProfile[]): string {
  const agent = agents.find((a) => a.id === agentId);
  if (!agent) return "?";
  return agent.name.slice(0, 2).toUpperCase();
}

function RecentTaskRow({ task, agents }: { task: RecentTask; agents: AgentProfile[] }) {
  const initials = task.assignee_agent_profile_id
    ? resolveAgentInitials(task.assignee_agent_profile_id, agents)
    : null;

  return (
    <Link
      href={`/office/tasks/${task.id}`}
      className="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors cursor-pointer"
    >
      <StatusIcon status={task.status} className="h-3.5 w-3.5" />
      <span className="font-mono text-xs text-muted-foreground shrink-0 w-14 truncate">
        {task.identifier}
      </span>
      <span className="flex-1 min-w-0 truncate">{task.title}</span>
      {initials && (
        <span className="h-5 w-5 rounded-full bg-muted flex items-center justify-center text-[10px] font-medium text-muted-foreground shrink-0">
          {initials}
        </span>
      )}
      <span className="text-xs text-muted-foreground shrink-0">{timeAgo(task.updated_at)}</span>
    </Link>
  );
}

function RecentTasksCard({ tasks, agents }: { tasks: RecentTask[]; agents: AgentProfile[] }) {
  return (
    <Card>
      <div className="p-4 border-b border-border">
        <h2 className="text-sm font-semibold">Recent Tasks</h2>
      </div>
      <div className="divide-y divide-border">
        {tasks.length === 0 ? (
          <div className="px-4 py-6 text-center text-sm text-muted-foreground">
            No recent tasks.
          </div>
        ) : (
          tasks.map((task) => <RecentTaskRow key={task.id} task={task} agents={agents} />)
        )}
      </div>
    </Card>
  );
}

function maxUtilization(agents: AgentProfile[]): number {
  let max = 0;
  for (const agent of agents) {
    if (agent.billingType !== "subscription" || !agent.utilization) continue;
    for (const w of agent.utilization.windows) {
      if (w.utilization_pct > max) max = w.utilization_pct;
    }
  }
  return max;
}

function SubscriptionUsageCard({ agents }: { agents: AgentProfile[] }) {
  const subscriptionAgents = agents.filter(
    (a) => a.billingType === "subscription" && a.utilization,
  );

  if (subscriptionAgents.length === 0) return null;

  return (
    <Card>
      <div className="p-4 border-b border-border">
        <h2 className="text-sm font-semibold">Subscription Quota</h2>
      </div>
      <div className="divide-y divide-border">
        {subscriptionAgents.map((agent) => (
          <div key={agent.id} className="px-4 py-3 space-y-2">
            <p className="text-xs font-medium text-muted-foreground">{agent.name}</p>
            {agent.utilization && <UtilizationBars usage={agent.utilization} />}
          </div>
        ))}
      </div>
    </Card>
  );
}

export function OfficePageClient({ initialDashboard }: OfficePageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const dashboard = useAppStore((s) => s.office.dashboard);
  const agents = useAppStore((s) => s.office.agentProfiles);
  const setDashboard = useAppStore((s) => s.setDashboard);

  // Hydrate from SSR exactly once on first mount; subsequent updates flow
  // through the WS-driven refetch below. Skipping the unconditional mount
  // fetch removes a redundant round-trip when SSR data is already in the
  // store (Stream G of office optimization).
  useEffect(() => {
    if (initialDashboard) {
      setDashboard(initialDashboard);
    }
  }, [initialDashboard, setDashboard]);

  const fetchDashboard = useCallback(async () => {
    if (!workspaceId) return;
    const data = await officeApi.getDashboard(workspaceId);
    setDashboard(data);
  }, [workspaceId, setDashboard]);

  // Refetch dashboard on any office event that affects metrics. The
  // dashboard payload now includes per-agent summaries so a single fetch
  // refreshes both the metric cards and the agent cards panel.
  useOfficeRefetch("dashboard", fetchDashboard);
  useOfficeRefetch("agents", fetchDashboard);

  const metrics = extractMetrics(dashboard);
  const topUtilization = maxUtilization(agents);
  const quotaLabel = topUtilization > 0 ? `${Math.round(topUtilization)}%` : "—";
  const hasSubscriptionAgents = agents.some((a) => a.billingType === "subscription");

  return (
    <div className="space-y-4 p-6">
      <AgentCardsPanel summaries={dashboard?.agent_summaries ?? []} />
      <MetricsGrid m={metrics} />
      {hasSubscriptionAgents && (
        <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
          <MetricCard
            icon={IconChartBar}
            value={quotaLabel}
            label="Subscription Quota"
            description="Highest utilization across subscription agents"
          />
        </div>
      )}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <RunActivityChart data={dashboard?.run_activity ?? []} />
        <SuccessRateChart data={dashboard?.run_activity ?? []} />
      </div>
      <div className="grid md:grid-cols-2 gap-4">
        <RecentActivityCard entries={metrics.recentActivity} />
        <div className="space-y-4">
          <RecentTasksCard tasks={dashboard?.recent_tasks ?? []} agents={agents} />
          <SubscriptionUsageCard agents={agents} />
          <ProviderHealthCard />
        </div>
      </div>
    </div>
  );
}
