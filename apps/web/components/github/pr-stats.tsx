"use client";

import { useEffect, useReducer } from "react";
import {
  IconGitPullRequest,
  IconEye,
  IconMessage,
  IconCheck,
  IconClock,
} from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { fetchGitHubStats } from "@/lib/api/domains/github-api";
import type { PRStats } from "@/lib/types/github";

type StatCardProps = {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  subtext?: string;
};

function StatCard({ icon, label, value, subtext }: StatCardProps) {
  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex items-center gap-3">
          <div className="text-muted-foreground">{icon}</div>
          <div>
            <p className="text-2xl font-bold">{value}</p>
            <p className="text-xs text-muted-foreground">{label}</p>
            {subtext && <p className="text-[10px] text-muted-foreground mt-0.5">{subtext}</p>}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function formatHours(hours: number): string {
  if (hours < 1) return "< 1h";
  if (hours < 24) return `${Math.round(hours)}h`;
  const days = Math.round(hours / 24);
  return `${days}d`;
}

function formatPercent(rate: number): string {
  return `${Math.round(rate * 100)}%`;
}

type StatsState = { stats: PRStats | null; loading: boolean };
type StatsAction = { type: "fetch" } | { type: "done"; stats: PRStats | null };

function statsReducer(state: StatsState, action: StatsAction): StatsState {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true };
    case "done":
      return { stats: action.stats, loading: false };
  }
}

function usePRStats(workspaceId: string | null) {
  const [state, dispatch] = useReducer(statsReducer, { stats: null, loading: true });

  useEffect(() => {
    let cancelled = false;
    dispatch({ type: "fetch" });
    fetchGitHubStats(workspaceId ? { workspace_id: workspaceId } : {}, { cache: "no-store" })
      .then((data) => {
        if (!cancelled) dispatch({ type: "done", stats: data });
      })
      .catch(() => {
        if (!cancelled) dispatch({ type: "done", stats: null });
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return state;
}

function StatsGrid({ stats }: { stats: PRStats }) {
  return (
    <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
      <StatCard
        icon={<IconGitPullRequest className="h-5 w-5" />}
        label="PRs Created"
        value={stats.total_prs_created}
      />
      <StatCard
        icon={<IconEye className="h-5 w-5" />}
        label="PRs Reviewed"
        value={stats.total_prs_reviewed}
      />
      <StatCard
        icon={<IconMessage className="h-5 w-5" />}
        label="Comments"
        value={stats.total_comments}
      />
      <StatCard
        icon={<IconCheck className="h-5 w-5" />}
        label="CI Pass Rate"
        value={formatPercent(stats.ci_pass_rate)}
      />
      <StatCard
        icon={<IconCheck className="h-5 w-5" />}
        label="Approval Rate"
        value={formatPercent(stats.approval_rate)}
      />
      <StatCard
        icon={<IconClock className="h-5 w-5" />}
        label="Avg Time to Merge"
        value={formatHours(stats.avg_time_to_merge_hours)}
      />
    </div>
  );
}

function PRsByDayChart({ prsByDay }: { prsByDay: PRStats["prs_by_day"] }) {
  if (!prsByDay || prsByDay.length === 0) return null;
  const maxCount = Math.max(...prsByDay.map((d) => d.count), 1);
  return (
    <Card>
      <CardContent className="p-4">
        <h4 className="text-sm font-medium mb-3">PRs by Day</h4>
        <div className="flex items-end gap-1 h-24">
          {prsByDay.map((day) => {
            const height = Math.max((day.count / maxCount) * 100, 4);
            return (
              <div
                key={day.date}
                className="flex-1 bg-primary/20 hover:bg-primary/40 rounded-t transition-colors"
                style={{ height: `${height}%` }}
                title={`${day.date}: ${day.count} PRs`}
              />
            );
          })}
        </div>
        <div className="flex justify-between mt-1">
          <span className="text-[10px] text-muted-foreground">{prsByDay[0]?.date}</span>
          <span className="text-[10px] text-muted-foreground">
            {prsByDay[prsByDay.length - 1]?.date}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}

export function PRStatsPanel({ workspaceId }: { workspaceId: string | null }) {
  const { stats, loading } = usePRStats(workspaceId);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  if (!stats) {
    return (
      <p className="text-sm text-muted-foreground text-center py-4">No stats available yet.</p>
    );
  }

  return (
    <div className="space-y-4">
      <StatsGrid stats={stats} />
      <PRsByDayChart prsByDay={stats.prs_by_day} />
    </div>
  );
}
