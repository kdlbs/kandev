"use client";

import { IconGitCommit } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { StatsResponse, TaskStatsDTO, RepositoryStatsDTO } from "@/lib/types/http";
import { formatDuration } from "./stats-utils";
import { useTranslation } from "react-i18next";

function formatPercent(value: number): string {
  return `${Math.round(value)}%`;
}

type GlobalStats = StatsResponse["global"];
type GitStats = StatsResponse["git_stats"];

function TasksCard({ global }: { global: GlobalStats }) {
  const { t } = useTranslation();
  const completionRate =
    global.total_tasks > 0 ? Math.round((global.completed_tasks / global.total_tasks) * 100) : 0;

  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("stats:tasks")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">{global.total_tasks}</div>
        <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
          <span>{t("stats:completedCount", { count: global.completed_tasks })}</span>
          <span>{t("stats:inProgressCount", { count: global.in_progress_tasks })}</span>
        </div>
        {global.total_tasks > 0 && (
          <div className="mt-3">
            <div className="flex justify-between text-xs mb-1">
              <span className="text-muted-foreground">{t("stats:completionRate")}</span>
              <span className="tabular-nums">{completionRate}%</span>
            </div>
            <div className="h-1.5 bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-emerald-500/70 rounded-full"
                style={{ width: `${completionRate}%` }}
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function TimeSpentCard({ global }: { global: GlobalStats }) {
  const { t } = useTranslation();
  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("stats:timeSpent")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">
          {formatDuration(global.total_duration_ms)}
        </div>
        <div className="mt-2 text-sm text-muted-foreground">
          {t("stats:avgPerTask", { duration: formatDuration(global.avg_duration_ms_per_task) })}
        </div>
        <div className="mt-3 grid grid-cols-2 gap-4 pt-3 border-t">
          <div>
            <div className="text-lg font-semibold tabular-nums">{global.total_turns}</div>
            <div className="text-xs text-muted-foreground">{t("stats:totalTurns")}</div>
          </div>
          <div>
            <div className="text-lg font-semibold tabular-nums">{global.total_messages}</div>
            <div className="text-xs text-muted-foreground">{t("stats:totalMessages")}</div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function GitOrAveragesCard({ global, git_stats }: { global: GlobalStats; git_stats?: GitStats }) {
  const { t } = useTranslation();
  const hasGitStats =
    git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);

  if (hasGitStats) {
    return (
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
            <IconGitCommit className="h-4 w-4" />
            {t("stats:gitActivity")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-3xl font-bold tabular-nums">{git_stats.total_commits}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {t("stats:filesChangedCount", { count: git_stats.total_files_changed })}
          </div>
          <div className="mt-3 flex items-center gap-4 pt-3 border-t text-sm">
            <span className="text-emerald-600 dark:text-emerald-400 tabular-nums">
              +{git_stats.total_insertions.toLocaleString()}
            </span>
            <span className="text-red-600 dark:text-red-400 tabular-nums">
              {"\u2212"}
              {git_stats.total_deletions.toLocaleString()}
            </span>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("stats:averages")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <div className="flex justify-between">
            <span className="text-sm text-muted-foreground">{t("stats:turnsPerTask")}</span>
            <span className="font-medium tabular-nums">{global.avg_turns_per_task.toFixed(1)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm text-muted-foreground">{t("stats:messagesPerTask")}</span>
            <span className="font-medium tabular-nums">
              {global.avg_messages_per_task.toFixed(1)}
            </span>
          </div>
          <div className="flex justify-between">
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="text-sm text-muted-foreground cursor-help">
                  {t("stats:turnDuration")}
                </span>
              </TooltipTrigger>
              <TooltipContent>{t("stats:meanDurationOfCompletedTurnsExcluding")}</TooltipContent>
            </Tooltip>
            <span className="font-medium tabular-nums">
              {formatDuration(global.avg_turn_duration_ms)}
            </span>
          </div>
          <div className="flex justify-between">
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="text-sm text-muted-foreground cursor-help">
                  {t("stats:messagesPerTurn")}
                </span>
              </TooltipTrigger>
              <TooltipContent>{t("stats:meanMessageCountPerTurnSame")}</TooltipContent>
            </Tooltip>
            <span className="font-medium tabular-nums">
              {global.avg_messages_per_turn === 0 ? "—" : global.avg_messages_per_turn.toFixed(1)}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm text-muted-foreground">{t("stats:sessions")}</span>
            <span className="font-medium tabular-nums">{global.total_sessions}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function SignalCard({ global }: { global: GlobalStats }) {
  const { t } = useTranslation();
  const avgTurnsPerSession =
    global.total_sessions > 0 ? global.total_turns / global.total_sessions : 0;
  const avgMessagesPerSession =
    global.total_sessions > 0 ? global.total_messages / global.total_sessions : 0;
  const toolShare =
    global.total_messages > 0
      ? Math.round((global.total_tool_calls / global.total_messages) * 100)
      : 0;
  const userShare =
    global.total_messages > 0
      ? Math.round((global.total_user_messages / global.total_messages) * 100)
      : 0;

  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("stats:signal")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">{global.total_sessions}</div>
        <div className="mt-2 text-sm text-muted-foreground">
          {t("stats:turnsMessagesPerSession", {
            turns: avgTurnsPerSession.toFixed(1),
            messages: avgMessagesPerSession.toFixed(1),
          })}
        </div>
        <div className="mt-3 grid grid-cols-2 gap-4 pt-3 border-t text-xs text-muted-foreground">
          <div className="space-y-1">
            <div className="flex justify-between">
              <span>{t("stats:userMsgs")}</span>
              <span className="tabular-nums font-mono">{global.total_user_messages}</span>
            </div>
            <div className="flex justify-between">
              <span>{t("stats:userShare")}</span>
              <span className="tabular-nums font-mono">{formatPercent(userShare)}</span>
            </div>
          </div>
          <div className="space-y-1">
            <div className="flex justify-between">
              <span>{t("stats:toolCalls")}</span>
              <span className="tabular-nums font-mono">{global.total_tool_calls}</span>
            </div>
            <div className="flex justify-between">
              <span>{t("stats:toolShare")}</span>
              <span className="tabular-nums font-mono">{formatPercent(toolShare)}</span>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function OverviewCards({
  global,
  git_stats,
}: {
  global: GlobalStats;
  git_stats?: GitStats;
}) {
  return (
    <div id="overview" className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 scroll-mt-24">
      <TasksCard global={global} />
      <TimeSpentCard global={global} />
      <GitOrAveragesCard global={global} git_stats={git_stats} />
      <SignalCard global={global} />
    </div>
  );
}

type WorkloadSectionProps = {
  task_stats: TaskStatsDTO[];
};

export function WorkloadSection({ task_stats }: WorkloadSectionProps) {
  const { t } = useTranslation();
  if (task_stats.length === 0) return null;

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {/* Longest Tasks (Most Complex) */}
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("stats:longestTasks")}
          </CardTitle>
          <p className="text-xs text-muted-foreground">{t("stats:rankedByActiveDuration")}</p>
        </CardHeader>
        <CardContent>
          <TaskDurationList
            tasks={task_stats}
            sortDirection="desc"
            emptyLabel={t("stats:noCompletedTasksYet")}
          />
        </CardContent>
      </Card>

      {/* Quickest Tasks */}
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("stats:quickestTasks")}
          </CardTitle>
          <p className="text-xs text-muted-foreground">{t("stats:rankedByActiveDuration")}</p>
        </CardHeader>
        <CardContent>
          <TaskDurationList
            tasks={task_stats}
            sortDirection="asc"
            emptyLabel={t("stats:noCompletedTasksYet")}
          />
        </CardContent>
      </Card>
    </div>
  );
}

type TaskDurationListProps = {
  tasks: TaskStatsDTO[];
  sortDirection: "asc" | "desc";
  emptyLabel: string;
};

export function RepositoryStatsGrid({
  repositoryStats,
}: {
  repositoryStats: RepositoryStatsDTO[];
}) {
  const { t } = useTranslation();
  if (!repositoryStats || repositoryStats.length === 0) {
    return (
      <div className="text-sm text-muted-foreground py-4">{t("stats:noRepositoryStatsYet")}</div>
    );
  }

  return (
    <div className="grid gap-3 md:grid-cols-2">
      {repositoryStats.map((repo) => {
        const completionRate =
          repo.total_tasks > 0 ? (repo.completed_tasks / repo.total_tasks) * 100 : 0;
        const hasGit = repo.total_commits > 0 || repo.total_files_changed > 0;

        return (
          <div key={repo.repository_id} className="rounded-sm border bg-muted/20 p-3">
            <div className="flex items-center justify-between gap-3">
              <div className="text-sm font-medium truncate" title={repo.repository_name}>
                {repo.repository_name}
              </div>
              <div className="text-xs text-muted-foreground tabular-nums font-mono">
                {formatDuration(repo.total_duration_ms)}
              </div>
            </div>

            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground font-mono">
              <span>{t("stats:repoTasksCount", { count: repo.total_tasks })}</span>
              <span>{t("stats:repoSessionsCount", { count: repo.session_count })}</span>
              <span>{t("stats:repoTurnsCount", { count: repo.turn_count })}</span>
              <span>{t("stats:repoMessagesCount", { count: repo.message_count })}</span>
            </div>

            <div className="mt-3">
              <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                <span>{t("stats:completion")}</span>
                <span className="tabular-nums font-mono">
                  {formatPercent(completionRate)} {"\u00B7"} {repo.completed_tasks}/
                  {repo.total_tasks}
                </span>
              </div>
            </div>

            <div className="mt-2 pt-2 border-t text-[11px] text-muted-foreground">
              {hasGit ? (
                <div className="flex items-center justify-between">
                  <span className="font-mono">{repo.total_commits} commits</span>
                  <span className="font-mono tabular-nums">
                    <span className="text-emerald-600 dark:text-emerald-400">
                      +{repo.total_insertions.toLocaleString()}
                    </span>{" "}
                    <span className="text-red-600 dark:text-red-400">
                      {"\u2212"}
                      {repo.total_deletions.toLocaleString()}
                    </span>
                  </span>
                </div>
              ) : (
                <div className="text-[11px] text-muted-foreground">
                  {t("stats:noGitActivityYet")}
                </div>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function RankedRepoList({
  repos,
  valueAccessor,
}: {
  repos: RepositoryStatsDTO[];
  valueAccessor: (repo: RepositoryStatsDTO) => string | number;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      {repos.length === 0 && (
        <div className="text-sm text-muted-foreground">{t("stats:noDataYet")}</div>
      )}
      {repos.map((repo, idx) => (
        <div key={repo.repository_id} className="flex items-center gap-3">
          <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate" title={repo.repository_name}>
              {repo.repository_name}
            </div>
          </div>
          <div className="text-sm font-medium tabular-nums font-mono">{valueAccessor(repo)}</div>
        </div>
      ))}
    </div>
  );
}

export function TopRepositories({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
  const { t } = useTranslation();
  if (!repositoryStats || repositoryStats.length === 0) {
    return (
      <div className="text-sm text-muted-foreground py-4">{t("stats:noRepositoryStatsYet")}</div>
    );
  }

  const topByTurns = [...repositoryStats]
    .filter((repo) => repo.turn_count > 0)
    .sort((a, b) => b.turn_count - a.turn_count)
    .slice(0, 3);

  const topByMessages = [...repositoryStats]
    .filter((repo) => repo.message_count > 0)
    .sort((a, b) => b.message_count - a.message_count)
    .slice(0, 3);

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          {t("stats:topByTurns")}
        </div>
        <RankedRepoList repos={topByTurns} valueAccessor={(r) => r.turn_count} />
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          {t("stats:topByMessages")}
        </div>
        <RankedRepoList repos={topByMessages} valueAccessor={(r) => r.message_count} />
      </div>
    </div>
  );
}

export function RepoLeaders({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
  const { t } = useTranslation();
  if (!repositoryStats || repositoryStats.length === 0) {
    return (
      <div className="text-sm text-muted-foreground py-4">{t("stats:noRepositoryStatsYet")}</div>
    );
  }

  const topByTasks = [...repositoryStats]
    .filter((repo) => repo.total_tasks > 0)
    .sort((a, b) => b.total_tasks - a.total_tasks)
    .slice(0, 3);

  const topByTime = [...repositoryStats]
    .filter((repo) => repo.total_duration_ms > 0)
    .sort((a, b) => b.total_duration_ms - a.total_duration_ms)
    .slice(0, 3);

  const topByCommits = [...repositoryStats]
    .filter((repo) => repo.total_commits > 0)
    .sort((a, b) => b.total_commits - a.total_commits)
    .slice(0, 3);

  return (
    <div className="grid gap-4 md:grid-cols-3">
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          {t("stats:mostTasks")}
        </div>
        <RankedRepoList repos={topByTasks} valueAccessor={(r) => r.total_tasks} />
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          {t("stats:mostTime")}
        </div>
        <RankedRepoList
          repos={topByTime}
          valueAccessor={(r) => formatDuration(r.total_duration_ms)}
        />
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          {t("stats:mostCommits")}
        </div>
        <RankedRepoList repos={topByCommits} valueAccessor={(r) => r.total_commits} />
      </div>
    </div>
  );
}

function TaskDurationList({ tasks, sortDirection, emptyLabel }: TaskDurationListProps) {
  const { t } = useTranslation();
  const filtered = [...tasks].filter((task) => task.active_duration_ms > 0);
  filtered.sort((a, b) =>
    sortDirection === "desc"
      ? b.active_duration_ms - a.active_duration_ms
      : a.active_duration_ms - b.active_duration_ms,
  );
  const top3 = filtered.slice(0, 3);

  return (
    <div className="space-y-3">
      {top3.map((task, idx) => (
        <div key={task.task_id} className="flex items-center gap-3">
          <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate" title={task.task_title}>
              {task.task_title}
            </div>
            <div className="text-xs text-muted-foreground">
              {t("stats:turnsMessagesMiddot", {
                turns: task.turn_count,
                messages: task.message_count,
              })}
            </div>
          </div>
          <div className="text-sm font-medium tabular-nums text-right">
            <div>{formatDuration(task.active_duration_ms)}</div>
            <div className="text-[11px] text-muted-foreground">
              {t("stats:durationSpan", { duration: formatDuration(task.elapsed_span_ms) })}
            </div>
          </div>
        </div>
      ))}
      {filtered.length === 0 && (
        <div className="text-sm text-muted-foreground py-2">{emptyLabel}</div>
      )}
    </div>
  );
}
