"use client";

import Link from "next/link";
import { IconArrowLeft } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { StatsResponse } from "@/lib/types/http";
import { useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
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

interface StatsPageClientProps {
  stats: StatsResponse | null;
  error: string | null;
  workspaceId?: string;
  activeRange?: RangeKey;
}

const EMPTY_STATS: StatsResponse = {
  global: {
    total_tasks: 0,
    completed_tasks: 0,
    in_progress_tasks: 0,
    total_sessions: 0,
    total_turns: 0,
    total_messages: 0,
    total_user_messages: 0,
    total_tool_calls: 0,
    total_duration_ms: 0,
    avg_turns_per_task: 0,
    avg_messages_per_task: 0,
    avg_duration_ms_per_task: 0,
  },
  task_stats: [],
  daily_activity: [],
  completed_activity: [],
  agent_usage: [],
  repository_stats: [],
  git_stats: { total_commits: 0, total_files_changed: 0, total_insertions: 0, total_deletions: 0 },
};

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
      <header className="flex items-center gap-3 p-4 pb-3">
        <Link href="/" className="text-2xl font-bold hover:opacity-80 cursor-pointer">
          KanDev
        </Link>
        <span className="text-muted-foreground">/</span>
        <span className="text-muted-foreground">Statistics</span>
      </header>
      <div className="flex-1 flex items-center justify-center">
        <p className="text-muted-foreground">{message}</p>
      </div>
    </div>
  );
}

type StatsHeaderProps = {
  global: StatsResponse["global"];
  range: RangeKey;
  copied: boolean;
  onRangeChange: (r: RangeKey) => void;
  onCopy: () => void;
};

function StatsHeader({ global, range, copied, onRangeChange, onCopy }: StatsHeaderProps) {
  return (
    <header className="flex items-center gap-3 p-4 pb-3 shrink-0">
      <Button variant="ghost" size="sm" asChild className="cursor-pointer">
        <Link href="/">
          <IconArrowLeft className="h-4 w-4" />
          Back
        </Link>
      </Button>
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Link href="/" className="font-semibold text-foreground hover:opacity-80 cursor-pointer">
          KanDev
        </Link>
        <span>›</span>
        <span>Statistics</span>
        <span className="text-muted-foreground/60">·</span>
        <span className="font-mono text-xs">
          {global.total_tasks} tasks · {global.total_sessions} sessions ·{" "}
          {formatDuration(global.total_duration_ms)}
        </span>
      </div>
      <div className="ml-auto flex items-center gap-2">
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
              className="cursor-pointer h-7 px-2 font-mono text-[11px] data-[state=on]:bg-muted data-[state=on]:text-foreground"
            >
              {getRangeLabel(key)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 px-2 font-mono text-[11px] cursor-pointer"
          onClick={onCopy}
        >
          {copied ? "Copied" : "Copy Stats"}
        </Button>
      </div>
    </header>
  );
}

type StatsContentProps = { resolvedStats: StatsResponse; rangeLabel: string };

function StatsContent({ resolvedStats, rangeLabel }: StatsContentProps) {
  const {
    global,
    task_stats,
    daily_activity,
    completed_activity,
    agent_usage,
    repository_stats,
    git_stats,
  } = resolvedStats;
  return (
    <div className="flex-1 overflow-auto">
      <div className="max-w-7xl mx-auto p-6">
        <div className="space-y-5">
          <OverviewCards global={global} git_stats={git_stats} />
          <div id="telemetry" className="flex items-center gap-3 pt-2 scroll-mt-24">
            <div className="text-[11px] uppercase tracking-wider text-muted-foreground">
              Telemetry
            </div>
            <div className="h-px flex-1 bg-border/60" />
          </div>
          <div id="completed" className="scroll-mt-24">
            <div className="grid gap-4 lg:grid-cols-3">
              <Card className="rounded-sm lg:col-span-2">
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Completed Tasks Over Time
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <CompletedTasksChart completedActivity={completed_activity} />
                </CardContent>
              </Card>
              <Card className="rounded-sm">
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Most Productive
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <MostProductiveSummary completedActivity={completed_activity} />
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
                <ActivityHeatmap dailyActivity={daily_activity} />
              </CardContent>
            </Card>
            <Card className="rounded-sm">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Top Agents
                </CardTitle>
              </CardHeader>
              <CardContent>
                <AgentUsageList agentUsage={agent_usage} />
              </CardContent>
            </Card>
          </div>
          <Card id="repositories" className="rounded-sm scroll-mt-24">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Repository Activity
              </CardTitle>
            </CardHeader>
            <CardContent>
              <RepositoryStatsGrid repositoryStats={repository_stats} />
            </CardContent>
          </Card>
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Top Repositories
              </CardTitle>
            </CardHeader>
            <CardContent>
              <TopRepositories repositoryStats={repository_stats} />
            </CardContent>
          </Card>
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Repo Leaders
              </CardTitle>
            </CardHeader>
            <CardContent>
              <RepoLeaders repositoryStats={repository_stats} />
            </CardContent>
          </Card>
          <div id="workload" className="flex items-center gap-3 pt-2 scroll-mt-24">
            <div className="text-[11px] uppercase tracking-wider text-muted-foreground">
              Workload
            </div>
            <div className="h-px flex-1 bg-border/60" />
          </div>
          <WorkloadSection task_stats={task_stats} />
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
    `*KanDev Stats — ${rangeLabel}*`,
    `- Tasks: ${global.total_tasks} total (${global.completed_tasks} done, ${global.in_progress_tasks} in progress) · ${completion} completion`,
    `- Completed (${rangeLabel}): ${completedInRange}`,
    `- Time: ${formatDuration(global.total_duration_ms)} total · ${formatDuration(global.avg_duration_ms_per_task)} avg/task`,
    `- Repos: ${repository_stats.length} tracked · Top repo: ${topRepoLabel}`,
    `- Git: ${gitLine}`,
  ].join("\n");
}

export function StatsPageClient({ stats, error, workspaceId, activeRange }: StatsPageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [copied, setCopied] = useState(false);
  const range = (activeRange ?? "month") as RangeKey;
  const rangeLabel = getRangeLabel(range);
  const resolvedStats = stats ?? EMPTY_STATS;

  const completedInRange = useMemo(
    () =>
      (resolvedStats.completed_activity ?? []).reduce((sum, item) => sum + item.completed_tasks, 0),
    [resolvedStats.completed_activity],
  );
  const statsSummary = useMemo(
    () => buildStatsSummary(resolvedStats, rangeLabel, completedInRange),
    [resolvedStats, rangeLabel, completedInRange],
  );

  if (!workspaceId) return <StatsEmptyState message="Select a workspace to view statistics." />;
  if (error)
    return (
      <div className="h-screen w-full flex flex-col bg-background">
        <header className="flex items-center gap-3 p-4 pb-3">
          <Link href="/" className="text-2xl font-bold hover:opacity-80 cursor-pointer">
            KanDev
          </Link>
          <span className="text-muted-foreground">/</span>
          <span className="text-muted-foreground">Statistics</span>
        </header>
        <div className="flex-1 flex items-center justify-center">
          <p className="text-destructive">Error loading stats: {error}</p>
        </div>
      </div>
    );
  if (!stats) return <StatsEmptyState message="No stats available." />;

  const handleCopyStats = async () => {
    try {
      await navigator.clipboard.writeText(statsSummary);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy stats summary", err);
    }
  };

  const handleRangeChange = (nextRange: RangeKey) => {
    const params = new URLSearchParams(searchParams?.toString() ?? "");
    params.set("range", nextRange);
    router.push(`/stats?${params.toString()}`);
  };

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <StatsHeader
        global={resolvedStats.global}
        range={range}
        copied={copied}
        onRangeChange={handleRangeChange}
        onCopy={handleCopyStats}
      />
      <StatsContent resolvedStats={resolvedStats} rangeLabel={rangeLabel} />
    </div>
  );
}
