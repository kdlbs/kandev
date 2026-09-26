"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { PageShell } from "@/components/page-shell";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { IconChartBar } from "@tabler/icons-react";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type {
  ModelUsageDTO,
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
  ModelUsageList,
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

const RANGE_LABEL_KEYS: Record<RangeKey, string> = {
  week: "stats:rangeLastWeek",
  month: "stats:rangeLastMonth",
  all: "stats:rangeAllTime",
};

function StatsEmptyState({ message }: { message: string }) {
  const { t } = useTranslation();
  return (
    <PageShell
      title={t("stats:statistics")}
      icon={<IconChartBar className="h-4 w-4" />}
      scroll="none"
    >
      <div className="flex min-h-0 flex-1 items-center justify-center bg-background">
        <p className="text-muted-foreground">{message}</p>
      </div>
    </PageShell>
  );
}

type StatsShellProps = {
  global: GlobalStatsDTO | null;
  range: RangeKey;
  copied: boolean;
  copyDisabled: boolean;
  hasError: boolean;
  onRangeChange: (r: RangeKey) => void;
  onCopy: () => void;
  children: React.ReactNode;
};

function StatsShell({
  global,
  range,
  copied,
  copyDisabled,
  hasError,
  onRangeChange,
  onCopy,
  children,
}: StatsShellProps) {
  const { t } = useTranslation();
  return (
    <PageShell
      title={t("stats:statistics")}
      icon={<IconChartBar className="h-4 w-4" />}
      subtitle={getSubtitle(global, hasError)}
      scroll="none"
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
                {t(RANGE_LABEL_KEYS[key])}
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
            {copied ? t("stats:copied") : t("stats:copyStats")}
          </Button>
        </>
      }
    >
      {children}
    </PageShell>
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

function ErrorPanel({
  title,
  message,
  onRetry,
  retrying = false,
}: {
  title: string;
  message: string;
  onRetry?: () => void;
  retrying?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Card className="rounded-sm" role="status" aria-live="polite">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">{message}</p>
        {onRetry && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3 min-h-11 cursor-pointer px-3 text-xs [@media(pointer:fine)]:h-7 [@media(pointer:fine)]:min-h-7"
            onClick={onRetry}
            disabled={retrying}
            aria-label={t("stats:retry")}
          >
            {t("stats:retry")}
          </Button>
        )}
        {retrying && (
          <span className="sr-only" role="status" aria-live="polite">
            {t("stats:retrying")}
          </span>
        )}
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
    onRetry?: () => void;
    showError?: boolean;
  },
): React.ReactNode {
  if (status.kind === "loading") return options.skeleton;
  if (status.kind === "error") {
    return (
      <>
        {options.showError !== false && (
          <ErrorPanel
            title={options.errorTitle}
            message={status.message}
            onRetry={options.onRetry}
            retrying={status.retrying}
          />
        )}
        {status.data !== undefined && options.ready(status.data)}
      </>
    );
  }
  return options.ready(status.data);
}

function OverviewPanel({
  global,
  git,
  onRetryGlobal,
  onRetryGit,
}: {
  global: SectionStatus<GlobalStatsDTO>;
  git: SectionStatus<GitStatsDTO>;
  onRetryGlobal: () => void;
  onRetryGit: () => void;
}) {
  const { t } = useTranslation();
  if (global.kind === "loading") return <OverviewCardsSkeleton />;
  // Render global cards as soon as `global` is ready; `git` is independent and
  // its failure must not blank the tasks/sessions/turns summary the user can
  // already see. OverviewCards.git_stats is optional → falls back to the
  // averages card when git data is missing.
  const gitData = git.kind === "loading" ? undefined : git.data;
  const cards = global.data ? <OverviewCards global={global.data} git_stats={gitData} /> : null;
  return (
    <>
      {global.kind === "error" && (
        <ErrorPanel
          title={t("stats:overview")}
          message={global.message}
          onRetry={onRetryGlobal}
          retrying={global.retrying}
        />
      )}
      {cards}
      {git.kind === "error" && (
        <ErrorPanel
          title={t("stats:gitActivity")}
          message={git.message}
          onRetry={onRetryGit}
          retrying={git.retrying}
        />
      )}
    </>
  );
}

function CompletedPanel({
  status,
  onRetry,
}: {
  status: SectionStatus<CompletedTaskActivityDTO[]>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return renderSection(status, {
    skeleton: (
      <div id="completed" className="scroll-mt-24">
        <ChartsSkeleton />
      </div>
    ),
    errorTitle: t("stats:completedTasksOverTime"),
    onRetry,
    ready: (data) => (
      <div id="completed" className="scroll-mt-24">
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="rounded-sm lg:col-span-2">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("stats:completedTasksOverTime")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <CompletedTasksChart completedActivity={data} />
            </CardContent>
          </Card>
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("stats:mostProductive")}
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
  models,
  rangeLabel,
  onRetryDaily,
  onRetryModels,
}: {
  daily: SectionStatus<DailyActivityDTO[]>;
  models: SectionStatus<ModelUsageDTO[]>;
  rangeLabel: string;
  onRetryDaily: () => void;
  onRetryModels: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div id="activity" className="grid gap-4 lg:grid-cols-2 scroll-mt-24">
      {renderSection(daily, {
        skeleton: <ActivitySkeleton />,
        errorTitle: t("stats:activity"),
        onRetry: onRetryDaily,
        ready: (data) => (
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("stats:activityRange", { range: rangeLabel.toLowerCase() })}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ActivityHeatmap dailyActivity={data} />
            </CardContent>
          </Card>
        ),
      })}
      {renderSection(models, {
        skeleton: <ActivitySkeleton />,
        errorTitle: t("stats:topModels"),
        onRetry: onRetryModels,
        ready: (data) => (
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("stats:topModels")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ModelUsageList modelUsage={data} />
            </CardContent>
          </Card>
        ),
      })}
    </div>
  );
}

function RepositoryActivityPanel({
  status,
  onRetry,
}: {
  status: SectionStatus<RepositoryStatsDTO[]>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return renderSection(status, {
    skeleton: <RepositoriesSkeleton />,
    errorTitle: t("stats:repositoryActivity"),
    onRetry,
    ready: (data) => (
      <Card id="repositories" className="rounded-sm scroll-mt-24">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("stats:repositoryActivity")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <RepositoryStatsGrid repositoryStats={data} />
        </CardContent>
      </Card>
    ),
  });
}

function TopRepositoriesPanel({
  status,
  onRetry,
}: {
  status: SectionStatus<RepositoryStatsDTO[]>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return renderSection(status, {
    skeleton: <TopRepositoriesSkeleton />,
    errorTitle: t("stats:topRepositories"),
    onRetry,
    showError: false,
    ready: (data) => (
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("stats:topRepositories")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <TopRepositories repositoryStats={data} />
        </CardContent>
      </Card>
    ),
  });
}

function RepoLeadersPanel({
  status,
  onRetry,
}: {
  status: SectionStatus<RepositoryStatsDTO[]>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return renderSection(status, {
    skeleton: <RepoLeadersSkeleton />,
    errorTitle: t("stats:repoLeaders"),
    onRetry,
    showError: false,
    ready: (data) => (
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("stats:repoLeaders")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <RepoLeaders repositoryStats={data} />
        </CardContent>
      </Card>
    ),
  });
}

function WorkloadPanel({
  status,
  onRetry,
}: {
  status: SectionStatus<TaskStatsDTO[]>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return renderSection(status, {
    skeleton: <WorkloadSkeleton />,
    errorTitle: t("stats:workload"),
    onRetry,
    ready: (data) => <WorkloadSection task_stats={data} />,
  });
}

function StatsContent({
  sections,
  rangeLabel,
  workspaceId,
  onRetrySection,
}: {
  sections: StatsSections;
  rangeLabel: string;
  workspaceId?: string;
  onRetrySection: (
    key: "global" | "tasks" | "daily" | "completed" | "models" | "repos" | "git",
  ) => void;
}) {
  const { t } = useTranslation();
  const taskStatus = flattenTaskStats(sections.tasks);
  return (
    <div className="min-h-0 flex-1 overflow-auto bg-background">
      <div className="max-w-7xl mx-auto p-6">
        <div className="space-y-5">
          <OverviewPanel
            global={sections.global}
            git={sections.git}
            onRetryGlobal={() => onRetrySection("global")}
            onRetryGit={() => onRetrySection("git")}
          />
          <SectionDivider id="telemetry" label={t("stats:telemetry")} />
          <CompletedPanel status={sections.completed} onRetry={() => onRetrySection("completed")} />
          <ActivityPanel
            daily={sections.daily}
            models={sections.models}
            rangeLabel={rangeLabel}
            onRetryDaily={() => onRetrySection("daily")}
            onRetryModels={() => onRetrySection("models")}
          />
          <RepositoryActivityPanel
            status={sections.repos}
            onRetry={() => onRetrySection("repos")}
          />
          <TopRepositoriesPanel status={sections.repos} onRetry={() => onRetrySection("repos")} />
          <RepoLeadersPanel status={sections.repos} onRetry={() => onRetrySection("repos")} />
          <SectionDivider id="github" label="GitHub" />
          <PRStatsPanel workspaceId={workspaceId ?? null} />
          <SectionDivider id="workload" label={t("stats:workload")} />
          <WorkloadPanel status={taskStatus} onRetry={() => onRetrySection("tasks")} />
        </div>
      </div>
    </div>
  );
}

export function StatsPageClient({ workspaceId, activeRange, initialError }: StatsPageClientProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { copied, copy } = useCopyToClipboard();

  const rawRange = searchParams?.get("range") ?? activeRange;
  const range: RangeKey = isRangeKey(rawRange) ? rawRange : DEFAULT_RANGE;
  // English label for the copyable stats summary (`buildStatsSummary`, in
  // stats-utils.ts — out of this migration's scope). The on-screen label is
  // `rangeLabelDisplay` below.
  const rangeLabel = getRangeLabel(range);
  const rangeLabelDisplay = t(RANGE_LABEL_KEYS[range]);

  const { retrySection, ...sections } = useStatsSections(workspaceId, range);
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
      <PageShell
        title={t("stats:statistics")}
        icon={<IconChartBar className="h-4 w-4" />}
        scroll="none"
      >
        <div className="flex min-h-0 flex-1 items-center justify-center bg-background">
          <p className="text-destructive">
            {t("stats:errorLoadingStats", { error: initialError })}
          </p>
        </div>
      </PageShell>
    );
  if (!workspaceId)
    return <StatsEmptyState message={t("stats:selectAWorkspaceToViewStatistics")} />;

  return (
    <StatsShell
      global={globalReady}
      range={range}
      copied={copied}
      copyDisabled={!fullStats}
      hasError={Boolean(fetchError)}
      onRangeChange={handleRangeChange}
      onCopy={handleCopyStats}
    >
      <StatsContent
        sections={sections}
        rangeLabel={rangeLabelDisplay}
        workspaceId={workspaceId}
        onRetrySection={retrySection}
      />
    </StatsShell>
  );
}
