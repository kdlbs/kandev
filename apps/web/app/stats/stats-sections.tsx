'use client';

import { IconGitCommit } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import type { StatsResponse, TaskStatsDTO, RepositoryStatsDTO } from '@/lib/types/http';

function formatDuration(ms: number): string {
  if (ms === 0) return '\u2014';
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h ${minutes % 60}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  }
  return `${seconds}s`;
}

function formatPercent(value: number): string {
  return `${Math.round(value)}%`;
}

type GlobalStats = StatsResponse['global'];
type GitStats = StatsResponse['git_stats'];

function TasksCard({ global }: { global: GlobalStats }) {
  const completionRate = global.total_tasks > 0
    ? Math.round((global.completed_tasks / global.total_tasks) * 100)
    : 0;

  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">Tasks</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">{global.total_tasks}</div>
        <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
          <span>{global.completed_tasks} completed</span>
          <span>{global.in_progress_tasks} in progress</span>
        </div>
        {global.total_tasks > 0 && (
          <div className="mt-3">
            <div className="flex justify-between text-xs mb-1">
              <span className="text-muted-foreground">Completion rate</span>
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
  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">Time Spent</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">{formatDuration(global.total_duration_ms)}</div>
        <div className="mt-2 text-sm text-muted-foreground">
          {formatDuration(global.avg_duration_ms_per_task)} avg per task
        </div>
        <div className="mt-3 grid grid-cols-2 gap-4 pt-3 border-t">
          <div>
            <div className="text-lg font-semibold tabular-nums">{global.total_turns}</div>
            <div className="text-xs text-muted-foreground">Total turns</div>
          </div>
          <div>
            <div className="text-lg font-semibold tabular-nums">{global.total_messages}</div>
            <div className="text-xs text-muted-foreground">Total messages</div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function GitOrAveragesCard({ global, git_stats }: { global: GlobalStats; git_stats: GitStats }) {
  const hasGitStats = git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);

  if (hasGitStats) {
    return (
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
            <IconGitCommit className="h-4 w-4" />
            Git Activity
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-3xl font-bold tabular-nums">{git_stats.total_commits}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {git_stats.total_files_changed} files changed
          </div>
          <div className="mt-3 flex items-center gap-4 pt-3 border-t text-sm">
            <span className="text-emerald-600 dark:text-emerald-400 tabular-nums">
              +{git_stats.total_insertions.toLocaleString()}
            </span>
            <span className="text-red-600 dark:text-red-400 tabular-nums">
              {'\u2212'}{git_stats.total_deletions.toLocaleString()}
            </span>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">Averages</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <div className="flex justify-between">
            <span className="text-sm text-muted-foreground">Turns per task</span>
            <span className="font-medium tabular-nums">{global.avg_turns_per_task.toFixed(1)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm text-muted-foreground">Messages per task</span>
            <span className="font-medium tabular-nums">{global.avg_messages_per_task.toFixed(1)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm text-muted-foreground">Sessions</span>
            <span className="font-medium tabular-nums">{global.total_sessions}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function SignalCard({ global }: { global: GlobalStats }) {
  const avgTurnsPerSession = global.total_sessions > 0
    ? global.total_turns / global.total_sessions
    : 0;
  const avgMessagesPerSession = global.total_sessions > 0
    ? global.total_messages / global.total_sessions
    : 0;
  const toolShare = global.total_messages > 0
    ? Math.round((global.total_tool_calls / global.total_messages) * 100)
    : 0;
  const userShare = global.total_messages > 0
    ? Math.round((global.total_user_messages / global.total_messages) * 100)
    : 0;

  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">Signal</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">
          {global.total_sessions}
        </div>
        <div className="mt-2 text-sm text-muted-foreground">
          {avgTurnsPerSession.toFixed(1)} turns {'\u00B7'} {avgMessagesPerSession.toFixed(1)} messages per session
        </div>
        <div className="mt-3 grid grid-cols-2 gap-4 pt-3 border-t text-xs text-muted-foreground">
          <div className="space-y-1">
            <div className="flex justify-between">
              <span>User msgs</span>
              <span className="tabular-nums font-mono">{global.total_user_messages}</span>
            </div>
            <div className="flex justify-between">
              <span>User share</span>
              <span className="tabular-nums font-mono">{formatPercent(userShare)}</span>
            </div>
          </div>
          <div className="space-y-1">
            <div className="flex justify-between">
              <span>Tool calls</span>
              <span className="tabular-nums font-mono">{global.total_tool_calls}</span>
            </div>
            <div className="flex justify-between">
              <span>Tool share</span>
              <span className="tabular-nums font-mono">{formatPercent(toolShare)}</span>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function OverviewCards({ global, git_stats }: { global: GlobalStats; git_stats: GitStats }) {
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
  if (task_stats.length === 0) return null;

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {/* Longest Tasks (Most Complex) */}
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Longest Tasks
          </CardTitle>
        </CardHeader>
        <CardContent>
          <TaskDurationList
            tasks={task_stats}
            sortDirection="desc"
            emptyLabel="No completed tasks yet."
          />
        </CardContent>
      </Card>

      {/* Quickest Tasks */}
      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Quickest Tasks
          </CardTitle>
        </CardHeader>
        <CardContent>
          <TaskDurationList
            tasks={task_stats}
            sortDirection="asc"
            emptyLabel="No completed tasks yet."
          />
        </CardContent>
      </Card>
    </div>
  );
}

type TaskDurationListProps = {
  tasks: TaskStatsDTO[];
  sortDirection: 'asc' | 'desc';
  emptyLabel: string;
};

export function RepositoryStatsGrid({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
  if (!repositoryStats || repositoryStats.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No repository stats yet.</div>;
  }

  return (
    <div className="grid gap-3 md:grid-cols-2">
      {repositoryStats.map((repo) => {
        const completionRate = repo.total_tasks > 0
          ? (repo.completed_tasks / repo.total_tasks) * 100
          : 0;
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
              <span>{repo.total_tasks} tasks</span>
              <span>{repo.session_count} sessions</span>
              <span>{repo.turn_count} turns</span>
              <span>{repo.message_count} msgs</span>
            </div>

            <div className="mt-3">
              <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                <span>Completion</span>
                <span className="tabular-nums font-mono">
                  {formatPercent(completionRate)} {'\u00B7'} {repo.completed_tasks}/{repo.total_tasks}
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
                    </span>{' '}
                    <span className="text-red-600 dark:text-red-400">
                      {'\u2212'}{repo.total_deletions.toLocaleString()}
                    </span>
                  </span>
                </div>
              ) : (
                <div className="text-[11px] text-muted-foreground">No git activity yet.</div>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function RankedRepoList({ repos, valueAccessor }: { repos: RepositoryStatsDTO[]; valueAccessor: (repo: RepositoryStatsDTO) => string | number }) {
  return (
    <div className="space-y-2">
      {repos.length === 0 && (
        <div className="text-sm text-muted-foreground">No data yet.</div>
      )}
      {repos.map((repo, idx) => (
        <div key={repo.repository_id} className="flex items-center gap-3">
          <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate" title={repo.repository_name}>
              {repo.repository_name}
            </div>
          </div>
          <div className="text-sm font-medium tabular-nums font-mono">
            {valueAccessor(repo)}
          </div>
        </div>
      ))}
    </div>
  );
}

export function TopRepositories({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
  if (!repositoryStats || repositoryStats.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No repository stats yet.</div>;
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
          Top By Turns
        </div>
        <RankedRepoList repos={topByTurns} valueAccessor={(r) => r.turn_count} />
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          Top By Messages
        </div>
        <RankedRepoList repos={topByMessages} valueAccessor={(r) => r.message_count} />
      </div>
    </div>
  );
}

export function RepoLeaders({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
  if (!repositoryStats || repositoryStats.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No repository stats yet.</div>;
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
          Most Tasks
        </div>
        <RankedRepoList repos={topByTasks} valueAccessor={(r) => r.total_tasks} />
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          Most Time
        </div>
        <RankedRepoList repos={topByTime} valueAccessor={(r) => formatDuration(r.total_duration_ms)} />
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          Most Commits
        </div>
        <RankedRepoList repos={topByCommits} valueAccessor={(r) => r.total_commits} />
      </div>
    </div>
  );
}

function TaskDurationList({ tasks, sortDirection, emptyLabel }: TaskDurationListProps) {
  const filtered = [...tasks].filter(t => t.total_duration_ms > 0);
  const sorted = sortDirection === 'desc'
    ? filtered.sort((a, b) => b.total_duration_ms - a.total_duration_ms)
    : filtered.sort((a, b) => a.total_duration_ms - b.total_duration_ms);
  const top3 = sorted.slice(0, 3);

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
              {task.turn_count} turns {'\u00B7'} {task.message_count} messages
            </div>
          </div>
          <div className="text-sm font-medium tabular-nums text-right">
            {formatDuration(task.total_duration_ms)}
          </div>
        </div>
      ))}
      {filtered.length === 0 && (
        <div className="text-sm text-muted-foreground py-2">{emptyLabel}</div>
      )}
    </div>
  );
}
