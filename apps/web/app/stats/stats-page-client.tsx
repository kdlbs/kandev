"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { PageTopbar } from "@/components/page-topbar";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { StatsResponse } from "@/lib/types/http";
import { useCallback, useEffect, useMemo, useReducer } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { IconChartBar } from "@tabler/icons-react";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { fetchStats } from "@/lib/api/domains/stats-api";
import {
  OverviewCards,
  WorkloadSection,
  RepositoryStatsGrid,
  TopRepositories,
  RepoLeaders,
} from "./stats-sections";
import {
  ActivityHeatmap,
  AgentUsageList,
  CompletedTasksChart,
  MostProductiveSummary,
} from "./stats-charts";
import {
  ActivitySkeleton,
  ChartsSkeleton,
  OverviewCardsSkeleton,
  RepoLeadersSkeleton,
  RepositoriesSkeleton,
  TopRepositoriesSkeleton,
  WorkloadSkeleton,
} from "./stats-skeletons";
import { PRStatsPanel } from "@/components/github/pr-stats";

interface StatsPageClientProps {
  workspaceId?: string;
  activeRange?: RangeKey;
  initialError?: string | null;
}

function formatDuration(ms: number): string {
  if (ms === 0) return "—";
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  if (hours > 0) return `${hours}h ${minutes % 60}m`;
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

type RangeKey = "week" | "month" | "all";

function getRangeLabel(range: RangeKey): string {
  switch (range) {
    case "week":
      return "Last Week";
    case "month":
      return "Last Month";
    case "all":
      return "All Time";
    default:
      return "Last Month";
  }
}

function StatsEmptyState({ message }: { message: string }) {
  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <PageTopbar title="Statistics" icon={<IconChartBar className="h-4 w-4" />} />
      <div className="flex-1 flex items-center justify-center">
        <p className="text-muted-foreground">{message}</p>
      </div>
    </div>
  );
}

type StatsHeaderProps = {
  global: StatsResponse["global"] | null;
  range: RangeKey;
  copied: boolean;
  copyDisabled: boolean;
  onRangeChange: (r: RangeKey) => void;
  onCopy: () => void;
};

function StatsHeader({
  global,
  range,
  copied,
  copyDisabled,
  onRangeChange,
  onCopy,
}: StatsHeaderProps) {
  const subtitle = global
    ? `${global.total_tasks} tasks · ${global.total_sessions} sessions · ${formatDuration(global.total_duration_ms)}`
    : "Loading stats…";
  return (
    <PageTopbar
      title="Statistics"
      icon={<IconChartBar className="h-4 w-4" />}
      subtitle={subtitle}
      actions={
        <>
          <ToggleGroup
            type="single"
            value={range}
            onValueChange={(v) => {
              if (v) onRangeChange(v as RangeKey);
            }}
            variant="outline"
            className="h-7"
          >
            {(["week", "month", "all"] as RangeKey[]).map((key) => (
              <ToggleGroupItem
                key={key}
                value={key}
                className="cursor-pointer h-7 px-2 text-xs data-[state=on]:bg-muted data-[state=on]:text-foreground"
              >
                {getRangeLabel(key)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 px-2 text-xs cursor-pointer"
            onClick={onCopy}
            disabled={copyDisabled}
          >
            {copied ? "Copied" : "Copy Stats"}
          </Button>
        </>
      }
    />
  );
}

function SectionDivider({ id, label }: { id: string; label: string }) {
  return (
    <div id={id} className="flex items-center gap-3 pt-2 scroll-mt-24">
      <div className="text-[11px] uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className="h-px flex-1 bg-border/60" />
    </div>
  );
}

type TelemetrySectionProps = {
  stats: StatsResponse | null;
  rangeLabel: string;
};

function TelemetrySection({ stats, rangeLabel }: TelemetrySectionProps) {
  if (!stats) {
    return (
      <>
        <div id="completed" className="scroll-mt-24">
          <ChartsSkeleton />
        </div>
        <div id="activity" className="scroll-mt-24">
          <ActivitySkeleton />
        </div>
      </>
    );
  }
  return (
    <>
      <div id="completed" className="scroll-mt-24">
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="rounded-sm lg:col-span-2">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Completed Tasks Over Time
              </CardTitle>
            </CardHeader>
            <CardContent>
              <CompletedTasksChart completedActivity={stats.completed_activity} />
            </CardContent>
          </Card>
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Most Productive
              </CardTitle>
            </CardHeader>
            <CardContent>
              <MostProductiveSummary completedActivity={stats.completed_activity} />
            </CardContent>
          </Card>
        </div>
      </div>
      <div id="activity" className="grid gap-4 lg:grid-cols-2 scroll-mt-24">
        <Card className="rounded-sm">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Activity ({rangeLabel.toLowerCase()})
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ActivityHeatmap dailyActivity={stats.daily_activity} />
          </CardContent>
        </Card>
        <Card className="rounded-sm">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Top Agents</CardTitle>
          </CardHeader>
          <CardContent>
            <AgentUsageList agentUsage={stats.agent_usage} />
          </CardContent>
        </Card>
      </div>
    </>
  );
}

type StatsContentProps = {
  stats: StatsResponse | null;
  rangeLabel: string;
  workspaceId?: string;
  fetchError: string | null;
};

function StatsContent({ stats, rangeLabel, workspaceId, fetchError }: StatsContentProps) {
  return (
    <div className="flex-1 overflow-auto">
      <div className="max-w-7xl mx-auto p-6">
        <div className="space-y-5">
          {fetchError && (
            <Card className="rounded-sm border-destructive/60">
              <CardContent className="py-3 text-sm text-destructive">
                Error loading stats: {fetchError}
              </CardContent>
            </Card>
          )}
          {stats ? (
            <OverviewCards global={stats.global} git_stats={stats.git_stats} />
          ) : (
            <OverviewCardsSkeleton />
          )}
          <SectionDivider id="telemetry" label="Telemetry" />
          <TelemetrySection stats={stats} rangeLabel={rangeLabel} />
          {stats ? (
            <Card id="repositories" className="rounded-sm scroll-mt-24">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Repository Activity
                </CardTitle>
              </CardHeader>
              <CardContent>
                <RepositoryStatsGrid repositoryStats={stats.repository_stats} />
              </CardContent>
            </Card>
          ) : (
            <RepositoriesSkeleton />
          )}
          {stats ? (
            <Card className="rounded-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Top Repositories
                </CardTitle>
              </CardHeader>
              <CardContent>
                <TopRepositories repositoryStats={stats.repository_stats} />
              </CardContent>
            </Card>
          ) : (
            <TopRepositoriesSkeleton />
          )}
          {stats ? (
            <Card className="rounded-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Repo Leaders
                </CardTitle>
              </CardHeader>
              <CardContent>
                <RepoLeaders repositoryStats={stats.repository_stats} />
              </CardContent>
            </Card>
          ) : (
            <RepoLeadersSkeleton />
          )}
          <SectionDivider id="github" label="GitHub" />
          <PRStatsPanel workspaceId={workspaceId ?? null} />
          <SectionDivider id="workload" label="Workload" />
          {stats ? <WorkloadSection task_stats={stats.task_stats} /> : <WorkloadSkeleton />}
        </div>
      </div>
    </div>
  );
}

function buildStatsSummary(
  resolvedStats: StatsResponse,
  rangeLabel: string,
  completedInRange: number,
): string {
  const { global, repository_stats, git_stats } = resolvedStats;
  const completion =
    global.total_tasks > 0
      ? `${Math.round((global.completed_tasks / global.total_tasks) * 100)}%`
      : "—";
  const topRepo = repository_stats
    .filter((r) => r.total_tasks > 0)
    .sort((a, b) => b.total_tasks - a.total_tasks)[0];
  const topRepoLabel = topRepo ? `${topRepo.repository_name} (${topRepo.total_tasks} tasks)` : "—";
  const hasGitStats =
    git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);
  const gitLine = hasGitStats
    ? `${git_stats.total_commits} commits, +${git_stats.total_insertions.toLocaleString()}/-${git_stats.total_deletions.toLocaleString()}`
    : "no git activity";
  return [
    `*Kandev Stats — ${rangeLabel}*`,
    `- Tasks: ${global.total_tasks} total (${global.completed_tasks} done, ${global.in_progress_tasks} in progress) · ${completion} completion`,
    `- Completed (${rangeLabel}): ${completedInRange}`,
    `- Time: ${formatDuration(global.total_duration_ms)} total · ${formatDuration(global.avg_duration_ms_per_task)} avg/task`,
    `- Repos: ${repository_stats.length} tracked · Top repo: ${topRepoLabel}`,
    `- Git: ${gitLine}`,
  ].join("\n");
}

type StatsState = {
  stats: StatsResponse | null;
  loading: boolean;
  error: string | null;
};

type StatsAction =
  | { type: "fetch" }
  | { type: "success"; stats: StatsResponse }
  | { type: "failure"; error: string };

function statsReducer(state: StatsState, action: StatsAction): StatsState {
  switch (action.type) {
    case "fetch":
      return { stats: null, loading: true, error: null };
    case "success":
      return { stats: action.stats, loading: false, error: null };
    case "failure":
      return { stats: null, loading: false, error: action.error };
  }
}

function useStatsData(workspaceId: string | undefined, range: RangeKey) {
  const [state, dispatch] = useReducer(statsReducer, {
    stats: null,
    loading: Boolean(workspaceId),
    error: null,
  });

  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    dispatch({ type: "fetch" });
    fetchStats(workspaceId, { cache: "no-store" }, range)
      .then((data) => {
        if (!cancelled) dispatch({ type: "success", stats: data });
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        const message = e instanceof Error ? e.message : "Failed to fetch stats";
        dispatch({ type: "failure", error: message });
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, range]);

  return state;
}

export function StatsPageClient({ workspaceId, activeRange, initialError }: StatsPageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { copied, copy } = useCopyToClipboard();
  const range = (activeRange ?? "month") as RangeKey;
  const rangeLabel = getRangeLabel(range);

  const { stats, error: fetchError } = useStatsData(workspaceId, range);

  const completedInRange = useMemo(
    () => (stats?.completed_activity ?? []).reduce((sum, item) => sum + item.completed_tasks, 0),
    [stats?.completed_activity],
  );
  const statsSummary = useMemo(
    () => (stats ? buildStatsSummary(stats, rangeLabel, completedInRange) : ""),
    [stats, rangeLabel, completedInRange],
  );

  const handleCopyStats = useCallback(() => {
    if (statsSummary) void copy(statsSummary);
  }, [copy, statsSummary]);

  const handleRangeChange = useCallback(
    (nextRange: RangeKey) => {
      const params = new URLSearchParams(searchParams?.toString() ?? "");
      params.set("range", nextRange);
      router.push(`/stats?${params.toString()}`);
    },
    [router, searchParams],
  );

  if (initialError)
    return (
      <div className="h-screen w-full flex flex-col bg-background">
        <PageTopbar title="Statistics" icon={<IconChartBar className="h-4 w-4" />} />
        <div className="flex-1 flex items-center justify-center">
          <p className="text-destructive">Error loading stats: {initialError}</p>
        </div>
      </div>
    );
  if (!workspaceId) return <StatsEmptyState message="Select a workspace to view statistics." />;

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <StatsHeader
        global={stats?.global ?? null}
        range={range}
        copied={copied}
        copyDisabled={!stats}
        onRangeChange={handleRangeChange}
        onCopy={handleCopyStats}
      />
      <StatsContent
        stats={stats}
        rangeLabel={rangeLabel}
        workspaceId={workspaceId}
        fetchError={fetchError}
      />
    </div>
  );
}
