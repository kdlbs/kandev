"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { PageTopbar } from "@/components/page-topbar";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { useCallback, useMemo } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { IconChartBar } from "@tabler/icons-react";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type {
  AgentUsageDTO,
  CompletedTaskActivityDTO,
  DailyActivityDTO,
  GitStatsDTO,
  GlobalStatsDTO,
  RepositoryStatsDTO,
  TaskStatsDTO,
} from "@/lib/types/http";
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
import {
  buildStatsSummary,
  DEFAULT_RANGE,
  getRangeLabel,
  getSubtitle,
  isRangeKey,
  RANGE_KEYS,
  type RangeKey,
} from "./stats-utils";
import {
  composeStatsResponse,
  firstError,
  flattenTaskStats,
  readyGlobal,
  type SectionStatus,
  type StatsSections,
  useStatsSections,
} from "./stats-data";

interface StatsPageClientProps {
  workspaceId?: string;
  activeRange?: RangeKey;
  initialError?: string | null;
}

function StatsEmptyState({ message }: { message: string }) {
  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <PageTopbar title="Statistics" icon={<IconChartBar className="h-4 w-4" />} />
      <div className="flex-1 flex items-center justify-center">
        <p className="text-muted-foreground">{message}</p>
      </div>
    </div>
  );
}

type StatsHeaderProps = {
  global: GlobalStatsDTO | null;
  range: RangeKey;
  copied: boolean;
  copyDisabled: boolean;
  hasError: boolean;
  onRangeChange: (r: RangeKey) => void;
  onCopy: () => void;
};

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

// renderSection picks a skeleton, error panel, or ready render based on
// the section's status. Centralising the switch keeps each call site to one line.
function renderSection<T>(
  status: SectionStatus<T>,
  options: {
    skeleton: React.ReactNode;
    errorTitle: string;
    ready: (data: T) => React.ReactNode;
  },
): React.ReactNode {
  if (status.kind === "loading") return options.skeleton;
  if (status.kind === "error")
    return <ErrorPanel title={options.errorTitle} message={status.message} />;
  return options.ready(status.data);
}

function OverviewPanel({
  global,
  git,
}: {
  global: SectionStatus<GlobalStatsDTO>;
  git: SectionStatus<GitStatsDTO>;
}) {
  if (global.kind === "loading") return <OverviewCardsSkeleton />;
  if (global.kind === "error") return <ErrorPanel title="Overview" message={global.message} />;
  // Render global cards as soon as `global` is ready; `git` is independent and
  // its failure must not blank the tasks/sessions/turns summary the user can
  // already see. OverviewCards.git_stats is optional → falls back to the
  // averages card when git data is missing.
  const gitData = git.kind === "ready" ? git.data : undefined;
  return <OverviewCards global={global.data} git_stats={gitData} />;
}

function CompletedPanel({ status }: { status: SectionStatus<CompletedTaskActivityDTO[]> }) {
  return renderSection(status, {
    skeleton: (
      <div id="completed" className="scroll-mt-24">
        <ChartsSkeleton />
      </div>
    ),
    errorTitle: "Completed Tasks Over Time",
    ready: (data) => (
      <div id="completed" className="scroll-mt-24">
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="rounded-sm lg:col-span-2">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Completed Tasks Over Time
              </CardTitle>
            </CardHeader>
            <CardContent>
              <CompletedTasksChart completedActivity={data} />
            </CardContent>
          </Card>
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Most Productive
              </CardTitle>
            </CardHeader>
            <CardContent>
              <MostProductiveSummary completedActivity={data} />
            </CardContent>
          </Card>
        </div>
      </div>
    ),
  });
}

function ActivityPanel({
  daily,
  agents,
  rangeLabel,
}: {
  daily: SectionStatus<DailyActivityDTO[]>;
  agents: SectionStatus<AgentUsageDTO[]>;
  rangeLabel: string;
}) {
  return (
    <div id="activity" className="grid gap-4 lg:grid-cols-2 scroll-mt-24">
      {renderSection(daily, {
        skeleton: <ActivitySkeleton />,
        errorTitle: "Activity",
        ready: (data) => (
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Activity ({rangeLabel.toLowerCase()})
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ActivityHeatmap dailyActivity={data} />
            </CardContent>
          </Card>
        ),
      })}
      {renderSection(agents, {
        skeleton: <ActivitySkeleton />,
        errorTitle: "Top Agents",
        ready: (data) => (
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Top Agents
              </CardTitle>
            </CardHeader>
            <CardContent>
              <AgentUsageList agentUsage={data} />
            </CardContent>
          </Card>
        ),
      })}
    </div>
  );
}

function RepositoryActivityPanel({ status }: { status: SectionStatus<RepositoryStatsDTO[]> }) {
  return renderSection(status, {
    skeleton: <RepositoriesSkeleton />,
    errorTitle: "Repository Activity",
    ready: (data) => (
      <Card id="repositories" className="rounded-sm scroll-mt-24">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Repository Activity
          </CardTitle>
        </CardHeader>
        <CardContent>
          <RepositoryStatsGrid repositoryStats={data} />
        </CardContent>
      </Card>
    ),
  });
}

function TopRepositoriesPanel({ status }: { status: SectionStatus<RepositoryStatsDTO[]> }) {
  return renderSection(status, {
    skeleton: <TopRepositoriesSkeleton />,
    errorTitle: "Top Repositories",
    ready: (data) => (
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Top Repositories
          </CardTitle>
        </CardHeader>
        <CardContent>
          <TopRepositories repositoryStats={data} />
        </CardContent>
      </Card>
    ),
  });
}

function RepoLeadersPanel({ status }: { status: SectionStatus<RepositoryStatsDTO[]> }) {
  return renderSection(status, {
    skeleton: <RepoLeadersSkeleton />,
    errorTitle: "Repo Leaders",
    ready: (data) => (
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Repo Leaders</CardTitle>
        </CardHeader>
        <CardContent>
          <RepoLeaders repositoryStats={data} />
        </CardContent>
      </Card>
    ),
  });
}

function WorkloadPanel({ status }: { status: SectionStatus<TaskStatsDTO[]> }) {
  return renderSection(status, {
    skeleton: <WorkloadSkeleton />,
    errorTitle: "Workload",
    ready: (data) => <WorkloadSection task_stats={data} />,
  });
}

function StatsContent({
  sections,
  rangeLabel,
  workspaceId,
}: {
  sections: StatsSections;
  rangeLabel: string;
  workspaceId?: string;
}) {
  const taskStatus = flattenTaskStats(sections.tasks);
  return (
    <div className="flex-1 overflow-auto">
      <div className="max-w-7xl mx-auto p-6">
        <div className="space-y-5">
          <OverviewPanel global={sections.global} git={sections.git} />
          <SectionDivider id="telemetry" label="Telemetry" />
          <CompletedPanel status={sections.completed} />
          <ActivityPanel daily={sections.daily} agents={sections.agents} rangeLabel={rangeLabel} />
          <RepositoryActivityPanel status={sections.repos} />
          <TopRepositoriesPanel status={sections.repos} />
          <RepoLeadersPanel status={sections.repos} />
          <SectionDivider id="github" label="GitHub" />
          <PRStatsPanel workspaceId={workspaceId ?? null} />
          <SectionDivider id="workload" label="Workload" />
          <WorkloadPanel status={taskStatus} />
        </div>
      </div>
    </div>
  );
}

export function StatsPageClient({ workspaceId, activeRange, initialError }: StatsPageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { copied, copy } = useCopyToClipboard();

  const rawRange = searchParams?.get("range") ?? activeRange;
  const range: RangeKey = isRangeKey(rawRange) ? rawRange : DEFAULT_RANGE;
  const rangeLabel = getRangeLabel(range);

  const sections = useStatsSections(workspaceId, range);
  const fetchError = firstError(sections);
  const globalReady = readyGlobal(sections);
  const fullStats = composeStatsResponse(sections);

  const completedInRange = useMemo(() => {
    if (sections.completed.kind !== "ready") return 0;
    return sections.completed.data.reduce((sum, item) => sum + item.completed_tasks, 0);
  }, [sections.completed]);

  const statsSummary = useMemo(
    () => (fullStats ? buildStatsSummary(fullStats, rangeLabel, completedInRange) : ""),
    [fullStats, rangeLabel, completedInRange],
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
      <div className="flex h-full min-h-0 w-full flex-col bg-background">
        <PageTopbar title="Statistics" icon={<IconChartBar className="h-4 w-4" />} />
        <div className="flex-1 flex items-center justify-center">
          <p className="text-destructive">Error loading stats: {initialError}</p>
        </div>
      </div>
    );
  if (!workspaceId) return <StatsEmptyState message="Select a workspace to view statistics." />;

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <StatsHeader
        global={globalReady}
        range={range}
        copied={copied}
        copyDisabled={!fullStats}
        hasError={Boolean(fetchError)}
        onRangeChange={handleRangeChange}
        onCopy={handleCopyStats}
      />
      <StatsContent sections={sections} rangeLabel={rangeLabel} workspaceId={workspaceId} />
    </div>
  );
}
