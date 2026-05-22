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

type RangeKey = "week" | "month" | "all";

const RANGE_KEYS = ["week", "month", "all"] as const;
const DEFAULT_RANGE: RangeKey = "month";

function isRangeKey(value: string | null | undefined): value is RangeKey {
  return value === "week" || value === "month" || value === "all";
}

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
  hasError: boolean;
  onRangeChange: (r: RangeKey) => void;
  onCopy: () => void;
};

function getSubtitle(global: StatsResponse["global"] | null, hasError: boolean): string {
  if (global) {
    return `${global.total_tasks} tasks · ${global.total_sessions} sessions · ${formatDuration(global.total_duration_ms)}`;
  }
  return hasError ? "Failed to load stats" : "Loading stats…";
}

function StatsHeader({
  global,
  range,
  copied,
  copyDisabled,
  hasError,
  onRangeChange,
  onCopy,
}: StatsHeaderProps) {
  return (
    <PageTopbar
      title="Statistics"
      icon={<IconChartBar className="h-4 w-4" />}
      subtitle={getSubtitle(global, hasError)}
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
            {RANGE_KEYS.map((key) => (
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

type PanelState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; stats: StatsResponse };

function ErrorPanel({ title, message }: { title: string; message: string }) {
  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">{message}</p>
      </CardContent>
    </Card>
  );
}

function TelemetryPanels({ state, rangeLabel }: { state: PanelState; rangeLabel: string }) {
  if (state.kind === "loading") {
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
  if (state.kind === "error") {
    return (
      <>
        <div id="completed" className="scroll-mt-24">
          <ErrorPanel title="Completed Tasks Over Time" message={state.message} />
        </div>
        <div id="activity" className="scroll-mt-24">
          <ErrorPanel title="Activity" message={state.message} />
        </div>
      </>
    );
  }
  const { stats } = state;
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

function renderOverviewPanel(state: PanelState) {
  if (state.kind === "loading") return <OverviewCardsSkeleton />;
  if (state.kind === "error") return <ErrorPanel title="Overview" message={state.message} />;
  return <OverviewCards global={state.stats.global} git_stats={state.stats.git_stats} />;
}

function renderRepositoryActivityPanel(state: PanelState) {
  if (state.kind === "loading") return <RepositoriesSkeleton />;
  if (state.kind === "error")
    return <ErrorPanel title="Repository Activity" message={state.message} />;
  return (
    <Card id="repositories" className="rounded-sm scroll-mt-24">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          Repository Activity
        </CardTitle>
      </CardHeader>
      <CardContent>
        <RepositoryStatsGrid repositoryStats={state.stats.repository_stats} />
      </CardContent>
    </Card>
  );
}

function renderTopRepositoriesPanel(state: PanelState) {
  if (state.kind === "loading") return <TopRepositoriesSkeleton />;
  if (state.kind === "error")
    return <ErrorPanel title="Top Repositories" message={state.message} />;
  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          Top Repositories
        </CardTitle>
      </CardHeader>
      <CardContent>
        <TopRepositories repositoryStats={state.stats.repository_stats} />
      </CardContent>
    </Card>
  );
}

function renderRepoLeadersPanel(state: PanelState) {
  if (state.kind === "loading") return <RepoLeadersSkeleton />;
  if (state.kind === "error") return <ErrorPanel title="Repo Leaders" message={state.message} />;
  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">Repo Leaders</CardTitle>
      </CardHeader>
      <CardContent>
        <RepoLeaders repositoryStats={state.stats.repository_stats} />
      </CardContent>
    </Card>
  );
}

function renderWorkloadPanel(state: PanelState) {
  if (state.kind === "loading") return <WorkloadSkeleton />;
  if (state.kind === "error") return <ErrorPanel title="Workload" message={state.message} />;
  return <WorkloadSection task_stats={state.stats.task_stats} />;
}

function StatsContent({
  state,
  rangeLabel,
  workspaceId,
}: {
  state: PanelState;
  rangeLabel: string;
  workspaceId?: string;
}) {
  return (
    <div className="flex-1 overflow-auto">
      <div className="max-w-7xl mx-auto p-6">
        <div className="space-y-5">
          {renderOverviewPanel(state)}
          <SectionDivider id="telemetry" label="Telemetry" />
          <TelemetryPanels state={state} rangeLabel={rangeLabel} />
          {renderRepositoryActivityPanel(state)}
          {renderTopRepositoriesPanel(state)}
          {renderRepoLeadersPanel(state)}
          <SectionDivider id="github" label="GitHub" />
          <PRStatsPanel workspaceId={workspaceId ?? null} />
          <SectionDivider id="workload" label="Workload" />
          {renderWorkloadPanel(state)}
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
  error: string | null;
};

type StatsAction =
  | { type: "fetch" }
  | { type: "success"; stats: StatsResponse }
  | { type: "failure"; error: string };

function statsReducer(state: StatsState, action: StatsAction): StatsState {
  switch (action.type) {
    case "fetch":
      return { stats: null, error: null };
    case "success":
      return { stats: action.stats, error: null };
    case "failure":
      return { stats: null, error: action.error };
  }
}

function useStatsData(workspaceId: string | undefined, range: RangeKey): StatsState {
  const [state, dispatch] = useReducer(statsReducer, { stats: null, error: null });

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

function toPanelState(stats: StatsResponse | null, error: string | null): PanelState {
  if (error) return { kind: "error", message: error };
  if (stats) return { kind: "ready", stats };
  return { kind: "loading" };
}

export function StatsPageClient({ workspaceId, activeRange, initialError }: StatsPageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { copied, copy } = useCopyToClipboard();

  const rawRange = searchParams?.get("range") ?? activeRange;
  const range: RangeKey = isRangeKey(rawRange) ? rawRange : DEFAULT_RANGE;
  const rangeLabel = getRangeLabel(range);

  const { stats, error: fetchError } = useStatsData(workspaceId, range);
  const panelState = toPanelState(stats, fetchError);

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
      router.replace(`/stats?${params.toString()}`, { scroll: false });
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
        hasError={Boolean(fetchError)}
        onRangeChange={handleRangeChange}
        onCopy={handleCopyStats}
      />
      <StatsContent state={panelState} rangeLabel={rangeLabel} workspaceId={workspaceId} />
    </div>
  );
}
