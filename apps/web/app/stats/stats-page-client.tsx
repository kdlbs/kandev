'use client';

import Link from 'next/link';
import { IconArrowLeft, IconGitCommit } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { ToggleGroup, ToggleGroupItem } from '@kandev/ui/toggle-group';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import type {
  StatsResponse,
  DailyActivityDTO,
  AgentUsageDTO,
  RepositoryStatsDTO,
  CompletedTaskActivityDTO,
} from '@/lib/types/http';
import { useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

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
  git_stats: {
    total_commits: 0,
    total_files_changed: 0,
    total_insertions: 0,
    total_deletions: 0,
  },
};

function formatDuration(ms: number): string {
  if (ms === 0) return '—';
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

type RangeKey = 'week' | 'month';

function getRangeLabel(range: RangeKey): string {
  switch (range) {
    case 'week':
      return 'Last Week';
    case 'month':
      return 'Last Month';
    default:
      return 'Last Month';
  }
}

function formatMonthLabel(date: Date): string {
  return date.toLocaleDateString('en-US', { month: 'short', year: '2-digit' });
}

function formatWeekLabel(date: Date): string {
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

function getHeatmapColor(intensity: number): string {
  if (intensity === 0) return 'bg-muted';
  if (intensity < 0.25) return 'bg-emerald-500/30';
  if (intensity < 0.5) return 'bg-emerald-500/50';
  if (intensity < 0.75) return 'bg-emerald-500/70';
  return 'bg-emerald-500/90';
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

interface HeatmapProps {
  dailyActivity: DailyActivityDTO[];
}

function ActivityHeatmap({ dailyActivity }: HeatmapProps) {
  const { weeks, maxActivity, monthLabels } = useMemo(() => {
    if (!dailyActivity || dailyActivity.length === 0) {
      return { weeks: [] as DailyActivityDTO[][], maxActivity: 1, monthLabels: [] as { index: number; label: string }[] };
    }

    const max = Math.max(...dailyActivity.map(d => d.turn_count + d.message_count), 1);
    const startDate = new Date(`${dailyActivity[0].date}T00:00:00`);
    const startDay = startDate.getDay();
    const padded: DailyActivityDTO[] = [];

    for (let i = 0; i < startDay; i++) {
      padded.push({ date: '', turn_count: 0, message_count: 0, task_count: 0 });
    }
    padded.push(...dailyActivity);

    const weeksMap: DailyActivityDTO[][] = [];
    for (let i = 0; i < padded.length; i += 7) {
      weeksMap.push(padded.slice(i, i + 7));
    }

    const monthMarkers: { index: number; label: string }[] = [];
    let lastMonth = -1;
    weeksMap.forEach((week, index) => {
      const firstDay = week.find((d) => d.date)?.date;
      if (!firstDay) return;
      const date = new Date(`${firstDay}T00:00:00`);
      const month = date.getMonth();
      if (month !== lastMonth) {
        monthMarkers.push({ index, label: date.toLocaleDateString('en-US', { month: 'short' }) });
        lastMonth = month;
      }
    });

    return { weeks: weeksMap, maxActivity: max, monthLabels: monthMarkers };
  }, [dailyActivity]);

  const dayLabels = ['', 'Mon', '', 'Wed', '', 'Fri', ''];

  return (
    <div className="overflow-x-auto">
      <div className="min-w-max">
        <div className="ml-8 h-4 flex items-end gap-3 text-[10px] text-muted-foreground">
          {monthLabels.map((label) => (
            <div key={`${label.label}-${label.index}`} style={{ marginLeft: `${label.index * 14}px` }}>
              {label.label}
            </div>
          ))}
        </div>

        <div className="flex gap-[3px]">
          <div className="flex flex-col gap-[3px] pr-2">
            {dayLabels.map((label, i) => (
              <div key={i} className="h-[10px] w-6 text-[10px] text-muted-foreground flex items-center">
                {label}
              </div>
            ))}
          </div>

          <TooltipProvider delayDuration={100}>
            <div className="flex gap-[3px]">
              {weeks.map((week, weekIndex) => (
                <div key={weekIndex} className="flex flex-col gap-[3px]">
                  {Array.from({ length: 7 }).map((_, dayIndex) => {
                    const day = week[dayIndex] ?? { date: '', turn_count: 0, message_count: 0, task_count: 0 };
                    const activity = day.turn_count + day.message_count;
                    const intensity = activity / maxActivity;

                    if (!day.date) {
                      return <div key={dayIndex} className="h-[10px] w-[10px]" />;
                    }

                    return (
                      <Tooltip key={day.date}>
                        <TooltipTrigger asChild>
                          <div className={`h-[10px] w-[10px] rounded-[2px] ${getHeatmapColor(intensity)}`} />
                        </TooltipTrigger>
                        <TooltipContent side="top" className="text-xs">
                          <div className="font-medium">{formatDate(day.date)}</div>
                          <div className="text-muted-foreground">
                            {day.turn_count} turns, {day.message_count} messages
                          </div>
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              ))}
            </div>
          </TooltipProvider>
        </div>
      </div>

      <div className="flex items-center gap-1 mt-2 text-[10px] text-muted-foreground">
        <span>Less</span>
        <div className="flex gap-[2px]">
          <div className="h-[10px] w-[10px] rounded-[2px] bg-muted" />
          <div className="h-[10px] w-[10px] rounded-[2px] bg-emerald-500/30" />
          <div className="h-[10px] w-[10px] rounded-[2px] bg-emerald-500/50" />
          <div className="h-[10px] w-[10px] rounded-[2px] bg-emerald-500/70" />
          <div className="h-[10px] w-[10px] rounded-[2px] bg-emerald-500/90" />
        </div>
        <span>More</span>
      </div>
    </div>
  );
}

function AgentUsageList({ agentUsage }: { agentUsage: AgentUsageDTO[] }) {
  if (!agentUsage || agentUsage.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No agent usage data yet.</div>;
  }

  const maxSessions = Math.max(...agentUsage.map((a) => a.session_count));

  return (
    <div className="space-y-3">
      {agentUsage.map((agent) => (
        <div key={agent.agent_profile_id}>
          <div className="flex items-center justify-between mb-1">
            <div className="min-w-0">
              <div className="text-sm truncate">{agent.agent_profile_name}</div>
              {agent.agent_model && (
                <div className="text-[11px] text-muted-foreground font-mono truncate">
                  {agent.agent_model}
                </div>
              )}
            </div>
            <span className="text-xs text-muted-foreground tabular-nums ml-2">
              {agent.session_count}
            </span>
          </div>
          <div className="h-1.5 bg-muted rounded-full overflow-hidden">
            <div
              className="h-full bg-primary/60 rounded-full"
              style={{ width: `${(agent.session_count / maxSessions) * 100}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

function RepositoryStatsGrid({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
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
                  {formatPercent(completionRate)} · {repo.completed_tasks}/{repo.total_tasks}
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
                      −{repo.total_deletions.toLocaleString()}
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

function TopRepositories({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
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
        <div className="space-y-2">
          {topByTurns.length === 0 && (
            <div className="text-sm text-muted-foreground">No turn activity yet.</div>
          )}
          {topByTurns.map((repo, idx) => (
            <div key={repo.repository_id} className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate" title={repo.repository_name}>
                  {repo.repository_name}
                </div>
              </div>
              <div className="text-sm font-medium tabular-nums font-mono">
                {repo.turn_count}
              </div>
            </div>
          ))}
        </div>
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          Top By Messages
        </div>
        <div className="space-y-2">
          {topByMessages.length === 0 && (
            <div className="text-sm text-muted-foreground">No message activity yet.</div>
          )}
          {topByMessages.map((repo, idx) => (
            <div key={repo.repository_id} className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate" title={repo.repository_name}>
                  {repo.repository_name}
                </div>
              </div>
              <div className="text-sm font-medium tabular-nums font-mono">
                {repo.message_count}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function RepoLeaders({ repositoryStats }: { repositoryStats: RepositoryStatsDTO[] }) {
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
        <div className="space-y-2">
          {topByTasks.length === 0 && (
            <div className="text-sm text-muted-foreground">No tasks yet.</div>
          )}
          {topByTasks.map((repo, idx) => (
            <div key={repo.repository_id} className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate" title={repo.repository_name}>
                  {repo.repository_name}
                </div>
              </div>
              <div className="text-sm font-medium tabular-nums font-mono">
                {repo.total_tasks}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          Most Time
        </div>
        <div className="space-y-2">
          {topByTime.length === 0 && (
            <div className="text-sm text-muted-foreground">No time logged yet.</div>
          )}
          {topByTime.map((repo, idx) => (
            <div key={repo.repository_id} className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate" title={repo.repository_name}>
                  {repo.repository_name}
                </div>
              </div>
              <div className="text-sm font-medium tabular-nums font-mono">
                {formatDuration(repo.total_duration_ms)}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div>
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground mb-2">
          Most Commits
        </div>
        <div className="space-y-2">
          {topByCommits.length === 0 && (
            <div className="text-sm text-muted-foreground">No commits yet.</div>
          )}
          {topByCommits.map((repo, idx) => (
            <div key={repo.repository_id} className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate" title={repo.repository_name}>
                  {repo.repository_name}
                </div>
              </div>
              <div className="text-sm font-medium tabular-nums font-mono">
                {repo.total_commits}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

type CompletionBucket = 'day' | 'week' | 'month';

function CompletedTasksChart({ completedActivity }: { completedActivity: CompletedTaskActivityDTO[] }) {
  const [bucket, setBucket] = useState<CompletionBucket>('day');
  const safeCompleted = completedActivity ?? [];

  const series = useMemo(() => {
    if (safeCompleted.length === 0) {
      return [] as { label: string; count: number; date: Date }[];
    }

    const toDate = (dateStr: string) => {
      const [year, month, day] = dateStr.split('-').map(Number);
      return new Date(Date.UTC(year, month - 1, day));
    };

    const data = safeCompleted
      .map((item) => ({ date: toDate(item.date), count: item.completed_tasks }))
      .filter((item) => Number.isFinite(item.date.getTime()));

    const buckets = new Map<string, { label: string; count: number; date: Date }>();

    data.forEach((item) => {
      if (bucket === 'day') {
        const key = item.date.toISOString().slice(0, 10);
        buckets.set(key, { label: formatDate(key), count: item.count, date: item.date });
        return;
      }

      if (bucket === 'week') {
        const d = new Date(item.date);
        const day = d.getUTCDay();
        const diff = (day + 6) % 7;
        d.setUTCDate(d.getUTCDate() - diff);
        d.setUTCHours(0, 0, 0, 0);
        const key = d.toISOString().slice(0, 10);
        const existing = buckets.get(key);
        const nextCount = (existing?.count ?? 0) + item.count;
        buckets.set(key, { label: formatWeekLabel(d), count: nextCount, date: d });
        return;
      }

      const d = new Date(Date.UTC(item.date.getUTCFullYear(), item.date.getUTCMonth(), 1));
      const key = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`;
      const existing = buckets.get(key);
      const nextCount = (existing?.count ?? 0) + item.count;
      buckets.set(key, { label: formatMonthLabel(d), count: nextCount, date: d });
    });

    return Array.from(buckets.values()).sort((a, b) => a.date.getTime() - b.date.getTime());
  }, [bucket, safeCompleted]);

  const maxCount = Math.max(...series.map((item) => item.count), 1);

  if (safeCompleted.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No completed task data yet.</div>;
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span className="uppercase tracking-wider text-[10px]">Bucket</span>
        <Button
          type="button"
          size="sm"
          variant={bucket === 'day' ? 'secondary' : 'outline'}
          className="h-7 px-2 font-mono text-[11px] cursor-pointer"
          onClick={() => setBucket('day')}
        >
          Day
        </Button>
        <Button
          type="button"
          size="sm"
          variant={bucket === 'week' ? 'secondary' : 'outline'}
          className="h-7 px-2 font-mono text-[11px] cursor-pointer"
          onClick={() => setBucket('week')}
        >
          Week
        </Button>
        <Button
          type="button"
          size="sm"
          variant={bucket === 'month' ? 'secondary' : 'outline'}
          className="h-7 px-2 font-mono text-[11px] cursor-pointer"
          onClick={() => setBucket('month')}
        >
          Month
        </Button>
      </div>

      <div className="h-32 flex items-end gap-1">
        <TooltipProvider delayDuration={100}>
          {series.map((item, index) => {
            const height = Math.max(6, Math.round((item.count / maxCount) * 100));
            return (
              <Tooltip key={`${item.label}-${index}`}>
                <TooltipTrigger asChild>
                  <div
                    className="flex-1 rounded-[2px] bg-emerald-500/70 hover:bg-emerald-500/90 transition-colors"
                    style={{ height: `${height}%` }}
                  />
                </TooltipTrigger>
                <TooltipContent side="top" className="text-xs">
                  <div className="font-medium">{item.label}</div>
                  <div className="text-muted-foreground">{item.count} completed</div>
                </TooltipContent>
              </Tooltip>
            );
          })}
        </TooltipProvider>
      </div>

      <div className="flex items-center justify-between text-[10px] text-muted-foreground font-mono">
        <span>{series[0]?.label ?? ''}</span>
        <span>{series[series.length - 1]?.label ?? ''}</span>
      </div>
    </div>
  );
}

function MostProductiveSummary({ completedActivity }: { completedActivity: CompletedTaskActivityDTO[] }) {
  const safeCompleted = completedActivity ?? [];

  const stats = useMemo(() => {
    if (safeCompleted.length === 0) {
      return {
        maxWeekday: { idx: 0, value: 0 },
        maxMonth: { idx: 0, value: 0 },
        maxYear: { year: 0, value: 0 },
      };
    }

    const weekdayTotals = Array(7).fill(0);
    const monthTotals = Array(12).fill(0);
    const yearTotals = new Map<number, number>();

    safeCompleted.forEach((item) => {
      const [year, month, day] = item.date.split('-').map(Number);
      const date = new Date(Date.UTC(year, month - 1, day));
      weekdayTotals[date.getUTCDay()] += item.completed_tasks;
      monthTotals[date.getUTCMonth()] += item.completed_tasks;
      yearTotals.set(year, (yearTotals.get(year) ?? 0) + item.completed_tasks);
    });

    const maxWeekday = weekdayTotals.reduce((best, value, idx) => (
      value > best.value ? { idx, value } : best
    ), { idx: 0, value: weekdayTotals[0] ?? 0 });

    const maxMonth = monthTotals.reduce((best, value, idx) => (
      value > best.value ? { idx, value } : best
    ), { idx: 0, value: monthTotals[0] ?? 0 });

    let maxYear = { year: 0, value: 0 };
    yearTotals.forEach((value, year) => {
      if (value > maxYear.value) {
        maxYear = { year, value };
      }
    });

    return { maxWeekday, maxMonth, maxYear };
  }, [safeCompleted]);

  if (safeCompleted.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No completed task data yet.</div>;
  }

  const weekdayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">Best weekday</span>
        <span className="font-mono tabular-nums">
          {weekdayNames[stats.maxWeekday.idx]} · {stats.maxWeekday.value}
        </span>
      </div>
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">Best month</span>
        <span className="font-mono tabular-nums">
          {monthNames[stats.maxMonth.idx]} · {stats.maxMonth.value}
        </span>
      </div>
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">Best year</span>
        <span className="font-mono tabular-nums">
          {stats.maxYear.year || '—'} · {stats.maxYear.value}
        </span>
      </div>
    </div>
  );
}

export function StatsPageClient({ stats, error, workspaceId, activeRange }: StatsPageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [copied, setCopied] = useState(false);
  const range = (activeRange ?? 'month') as RangeKey;
  const rangeLabel = getRangeLabel(range);
  const resolvedStats = stats ?? EMPTY_STATS;

  const completedInRange = useMemo(() => (
    (resolvedStats.completed_activity ?? []).reduce((sum, item) => sum + item.completed_tasks, 0)
  ), [resolvedStats.completed_activity]);

  const statsSummary = useMemo(() => {
    const repoCount = resolvedStats.repository_stats.length;
    const completed = resolvedStats.global.completed_tasks;
    const inProgress = resolvedStats.global.in_progress_tasks;
    const total = resolvedStats.global.total_tasks;
    const completion = total > 0 ? `${Math.round((completed / total) * 100)}%` : '—';
    const time = formatDuration(resolvedStats.global.total_duration_ms);
    const avgTask = formatDuration(resolvedStats.global.avg_duration_ms_per_task);
    const topRepo = resolvedStats.repository_stats
      .filter((repo) => repo.total_tasks > 0)
      .sort((a, b) => b.total_tasks - a.total_tasks)[0];
    const topRepoLabel = topRepo ? `${topRepo.repository_name} (${topRepo.total_tasks} tasks)` : '—';
    const hasGitStats = resolvedStats.git_stats
      && (resolvedStats.git_stats.total_commits > 0 || resolvedStats.git_stats.total_files_changed > 0);
    const gitLine = hasGitStats
      ? `${resolvedStats.git_stats.total_commits} commits, +${resolvedStats.git_stats.total_insertions.toLocaleString()}/-${resolvedStats.git_stats.total_deletions.toLocaleString()}`
      : 'no git activity';

    return [
      `*KanDev Stats — ${rangeLabel}*`,
      `- Tasks: ${total} total (${completed} done, ${inProgress} in progress) · ${completion} completion`,
      `- Completed (${rangeLabel}): ${completedInRange}`,
      `- Time: ${time} total · ${avgTask} avg/task`,
      `- Repos: ${repoCount} tracked · Top repo: ${topRepoLabel}`,
      `- Git: ${gitLine}`,
    ].join('\n');
  }, [completedInRange, rangeLabel, resolvedStats]);

  // No workspace selected
  if (!workspaceId) {
    return (
      <div className="h-screen w-full flex flex-col bg-background">
        <header className="flex items-center gap-3 p-4 pb-3">
          <Link href="/" className="text-2xl font-bold hover:opacity-80 cursor-pointer">KanDev</Link>
          <span className="text-muted-foreground">/</span>
          <span className="text-muted-foreground">Statistics</span>
        </header>
        <div className="flex-1 flex items-center justify-center">
          <p className="text-muted-foreground">Select a workspace to view statistics.</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-screen w-full flex flex-col bg-background">
        <header className="flex items-center gap-3 p-4 pb-3">
          <Link href="/" className="text-2xl font-bold hover:opacity-80 cursor-pointer">KanDev</Link>
          <span className="text-muted-foreground">/</span>
          <span className="text-muted-foreground">Statistics</span>
        </header>
        <div className="flex-1 flex items-center justify-center">
          <p className="text-destructive">Error loading stats: {error}</p>
        </div>
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="h-screen w-full flex flex-col bg-background">
        <header className="flex items-center gap-3 p-4 pb-3">
          <Link href="/" className="text-2xl font-bold hover:opacity-80 cursor-pointer">KanDev</Link>
          <span className="text-muted-foreground">/</span>
          <span className="text-muted-foreground">Statistics</span>
        </header>
        <div className="flex-1 flex items-center justify-center">
          <p className="text-muted-foreground">No stats available.</p>
        </div>
      </div>
    );
  }

  const {
    global,
    task_stats,
    daily_activity,
    completed_activity,
    agent_usage,
    repository_stats,
    git_stats,
  } = resolvedStats;
  const completionRate = global.total_tasks > 0
    ? Math.round((global.completed_tasks / global.total_tasks) * 100)
    : 0;
  const hasGitStats = git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);
  const toolShare = global.total_messages > 0
    ? Math.round((global.total_tool_calls / global.total_messages) * 100)
    : 0;
  const userShare = global.total_messages > 0
    ? Math.round((global.total_user_messages / global.total_messages) * 100)
    : 0;
  const avgTurnsPerSession = global.total_sessions > 0
    ? global.total_turns / global.total_sessions
    : 0;
  const avgMessagesPerSession = global.total_sessions > 0
    ? global.total_messages / global.total_sessions
    : 0;
  const handleCopyStats = async () => {
    try {
      await navigator.clipboard.writeText(statsSummary);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy stats summary', err);
    }
  };

  const handleRangeChange = (nextRange: RangeKey) => {
    const params = new URLSearchParams(searchParams?.toString() ?? '');
    params.set('range', nextRange);
    router.push(`/stats?${params.toString()}`);
  };

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      {/* Header matching task session top bar */}
      <header className="flex items-center gap-3 p-4 pb-3 shrink-0">
        <Button variant="ghost" size="sm" asChild className="cursor-pointer">
          <Link href="/">
            <IconArrowLeft className="h-4 w-4" />
            Back
          </Link>
        </Button>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Link href="/" className="font-semibold text-foreground hover:opacity-80 cursor-pointer">KanDev</Link>
          <span>›</span>
          <span>Statistics</span>
          <span className="text-muted-foreground/60">·</span>
          <span className="font-mono text-xs">
            {global.total_tasks} tasks · {global.total_sessions} sessions · {formatDuration(global.total_duration_ms)}
          </span>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <ToggleGroup
            type="single"
            value={range}
            onValueChange={(value) => {
              if (value) handleRangeChange(value as RangeKey);
            }}
            variant="outline"
            className="h-7"
          >
            {(['week', 'month'] as RangeKey[]).map((key) => (
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
            onClick={handleCopyStats}
          >
            {copied ? 'Copied' : 'Copy Stats'}
          </Button>
        </div>
      </header>

      {/* Scrollable content area */}
      <div className="flex-1 overflow-auto">
        <div className="max-w-7xl mx-auto p-6">
          <div className="space-y-5">
          {/* Overview Cards */}
          <div id="overview" className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 scroll-mt-24">
        {/* Tasks Overview */}
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

        {/* Time Spent */}
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

        {/* Git Stats or Averages */}
        {hasGitStats ? (
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
                  −{git_stats.total_deletions.toLocaleString()}
                </span>
              </div>
            </CardContent>
          </Card>
        ) : (
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
        )}

        {/* Signal */}
        <Card className="rounded-sm">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Signal</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold tabular-nums">
              {global.total_sessions}
            </div>
            <div className="mt-2 text-sm text-muted-foreground">
              {avgTurnsPerSession.toFixed(1)} turns · {avgMessagesPerSession.toFixed(1)} messages per session
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
      </div>

      <div id="telemetry" className="flex items-center gap-3 pt-2 scroll-mt-24">
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground">Telemetry</div>
        <div className="h-px flex-1 bg-border/60" />
      </div>

      <div id="completed" className="scroll-mt-24">
      {/* Completed Tasks and Productivity */}
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

      {/* Activity and Agents Row */}
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
            <CardTitle className="text-sm font-medium text-muted-foreground">Top Agents</CardTitle>
          </CardHeader>
          <CardContent>
            <AgentUsageList agentUsage={agent_usage} />
          </CardContent>
        </Card>
      </div>

      {/* Repositories */}
      <Card id="repositories" className="rounded-sm scroll-mt-24">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Repository Activity</CardTitle>
        </CardHeader>
        <CardContent>
          <RepositoryStatsGrid repositoryStats={repository_stats} />
        </CardContent>
      </Card>

      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Top Repositories</CardTitle>
        </CardHeader>
        <CardContent>
          <TopRepositories repositoryStats={repository_stats} />
        </CardContent>
      </Card>

      <Card className="rounded-sm">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Repo Leaders</CardTitle>
        </CardHeader>
        <CardContent>
          <RepoLeaders repositoryStats={repository_stats} />
        </CardContent>
      </Card>

      <div id="workload" className="flex items-center gap-3 pt-2 scroll-mt-24">
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground">Workload</div>
        <div className="h-px flex-1 bg-border/60" />
      </div>

      {/* Top Tasks by Duration */}
      {task_stats.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-2">
          {/* Longest Tasks (Most Complex) */}
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Longest Tasks
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                {[...task_stats]
                  .filter(t => t.total_duration_ms > 0)
                  .sort((a, b) => b.total_duration_ms - a.total_duration_ms)
                  .slice(0, 3)
                  .map((task, idx) => (
                    <div key={task.task_id} className="flex items-center gap-3">
                      <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium truncate" title={task.task_title}>
                          {task.task_title}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {task.turn_count} turns · {task.message_count} messages
                        </div>
                      </div>
                      <div className="text-sm font-medium tabular-nums text-right">
                        {formatDuration(task.total_duration_ms)}
                      </div>
                    </div>
                  ))}
                {task_stats.filter(t => t.total_duration_ms > 0).length === 0 && (
                  <div className="text-sm text-muted-foreground py-2">No completed tasks yet.</div>
                )}
              </div>
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
              <div className="space-y-3">
                {[...task_stats]
                  .filter(t => t.total_duration_ms > 0)
                  .sort((a, b) => a.total_duration_ms - b.total_duration_ms)
                  .slice(0, 3)
                  .map((task, idx) => (
                    <div key={task.task_id} className="flex items-center gap-3">
                      <span className="text-xs text-muted-foreground w-4">{idx + 1}.</span>
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium truncate" title={task.task_title}>
                          {task.task_title}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {task.turn_count} turns · {task.message_count} messages
                        </div>
                      </div>
                      <div className="text-sm font-medium tabular-nums text-right">
                        {formatDuration(task.total_duration_ms)}
                      </div>
                    </div>
                  ))}
                {task_stats.filter(t => t.total_duration_ms > 0).length === 0 && (
                  <div className="text-sm text-muted-foreground py-2">No completed tasks yet.</div>
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      )}
          </div>
        </div>
      </div>
    </div>
  );
}
