"use client";

import { useCallback, useEffect, useRef } from "react";
import Link from "@/components/routing/app-link";
import {
  IconRobot,
  IconCircleDot,
  IconCurrencyDollar,
  IconShieldCheck,
  IconChartBar,
} from "@tabler/icons-react";
import { Card } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import {
  selectOfficeAgentProfiles,
  selectOfficeDashboard,
} from "@/lib/state/slices/office/selectors";
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
import { timeAgo } from "@/lib/utils/time";

import { UtilizationBars } from "./components/utilization-bars";
import { formatDollars } from "@/lib/utils";
import { useTranslation } from "react-i18next";

// formatMonthSpend renders the subcents value from /office dashboard
// as USD. The shared formatDollars helper owns the unit boundary; this
// is a local alias for readability.
function formatMonthSpend(subcents: number): string {
  return formatDollars(subcents);
}

type OfficePageClientProps = {
  initialDashboard?: DashboardData | null;
  initialWorkspaceId?: string | null;
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
  const { t } = useTranslation();
  const tb = m.taskBreakdown;
  return (
    <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
      <Link href="/office/agents" className="cursor-pointer">
        <MetricCard
          icon={IconRobot}
          value={m.agentCount}
          label={t("office:agentsEnabled")}
          description={t("office:runningPausedErrors", {
            running: m.running,
            paused: m.paused,
            errors: m.errors,
          })}
        />
      </Link>
      <Link href="/office/tasks" className="cursor-pointer">
        <MetricCard
          icon={IconCircleDot}
          value={m.tasksInProgress}
          label={t("office:tasksInProgress")}
          description={t("office:openBlocked", { open: tb.open, blocked: tb.blocked })}
        />
      </Link>
      <Link href="/office/workspace/costs" className="cursor-pointer">
        <MetricCard
          icon={IconCurrencyDollar}
          value={formatMonthSpend(m.monthSpend)}
          label={t("office:monthSpend")}
          description={t("office:totalApiCostsThisBillingPeriod")}
        />
      </Link>
      <Link href="/office/inbox" className="cursor-pointer">
        <MetricCard
          icon={IconShieldCheck}
          value={m.pendingApprovals}
          label={t("office:pendingApprovals")}
          description={t("office:itemsWaitingForYourReview")}
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
  const { t } = useTranslation();
  return (
    <Card>
      <div className="p-4 border-b border-border">
        <h2 className="text-sm font-semibold">{t("office:recentActivity")}</h2>
      </div>
      <div className="divide-y divide-border">
        {entries.length === 0 ? (
          <div className="px-4 py-6 text-center text-sm text-muted-foreground">
            {t("office:noRecentActivityActionsByAgents")}
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
      className="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-muted/50 transition-colors cursor-pointer"
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
  const { t } = useTranslation();
  return (
    <Card>
      <div className="p-4 border-b border-border">
        <h2 className="text-sm font-semibold">{t("office:recentTasks")}</h2>
      </div>
      <div className="divide-y divide-border">
        {tasks.length === 0 ? (
          <div className="px-4 py-6 text-center text-sm text-muted-foreground">
            {t("office:noRecentTasks")}
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
  const { t } = useTranslation();
  const subscriptionAgents = agents.filter(
    (a) => a.billingType === "subscription" && a.utilization,
  );

  if (subscriptionAgents.length === 0) return null;

  return (
    <Card>
      <div className="p-4 border-b border-border">
        <h2 className="text-sm font-semibold">{t("office:subscriptionQuota")}</h2>
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

export function OfficePageClient({ initialDashboard, initialWorkspaceId }: OfficePageClientProps) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const dashboard = useAppStore(selectOfficeDashboard);
  const agents = useAppStore(selectOfficeAgentProfiles);
  const setDashboard = useAppStore((s) => s.setDashboard);
  const dashboardWorkspaceIdRef = useRef<string | null>(
    (dashboard || initialDashboard) && workspaceId ? workspaceId : null,
  );

  // Hydrate from SSR exactly once on first mount; subsequent updates flow
  // through the WS-driven refetch below. Skipping the unconditional mount
  // fetch removes a redundant round-trip when SSR data is already in the
  // store (Stream G of office optimization). Once means once: the payload
  // belongs to the workspace that was active at SSR time, and re-running on a
  // workspace switch would file it under the new workspace.
  const initialDashboardHydratedRef = useRef(false);
  useEffect(() => {
    if (
      initialDashboardHydratedRef.current ||
      !workspaceId ||
      !initialDashboard ||
      (initialWorkspaceId !== undefined && initialWorkspaceId !== workspaceId)
    ) {
      return;
    }
    initialDashboardHydratedRef.current = true;
    setDashboard(workspaceId, initialDashboard);
  }, [initialDashboard, initialWorkspaceId, setDashboard, workspaceId]);

  const fetchDashboard = useCallback(async () => {
    if (!workspaceId) return;
    const data = await officeApi.getDashboard(workspaceId);
    setDashboard(workspaceId, data);
    dashboardWorkspaceIdRef.current = workspaceId;
  }, [workspaceId, setDashboard]);

  useEffect(() => {
    if (!workspaceId || dashboardWorkspaceIdRef.current === workspaceId) return;
    dashboardWorkspaceIdRef.current = workspaceId;
    void fetchDashboard().catch(() => {
      if (dashboardWorkspaceIdRef.current === workspaceId) {
        dashboardWorkspaceIdRef.current = null;
      }
    });
  }, [fetchDashboard, workspaceId]);

  // Refetch dashboard on any office event that affects metrics. The
  // dashboard payload now includes per-agent summaries so a single fetch
  // refreshes both the metric cards and the agent cards panel.
  useOfficeRefetch("dashboard", fetchDashboard);
  useOfficeRefetch("agents", fetchDashboard);

  const metrics = extractMetrics(dashboard);
  const topUtilization = maxUtilization(agents);
  const quotaLabel = topUtilization > 0 ? `${Math.round(topUtilization)}%` : "-";
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
            label={t("office:subscriptionQuota")}
            description={t("office:highestUtilizationAcrossSubscriptionAgents")}
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
