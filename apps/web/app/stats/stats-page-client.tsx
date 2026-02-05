'use client';

import Link from 'next/link';
import { IconGitCommit, IconArrowLeft } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@kandev/ui/table';
import { Badge } from '@kandev/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import type { StatsResponse, DailyActivityDTO, AgentUsageDTO } from '@/lib/types/http';
import { useMemo } from 'react';

interface StatsPageClientProps {
  stats: StatsResponse | null;
  error: string | null;
  workspaceId?: string;
}

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

function getStateBadgeVariant(state: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (state) {
    case 'COMPLETED':
      return 'default';
    case 'IN_PROGRESS':
      return 'secondary';
    case 'FAILED':
    case 'CANCELLED':
      return 'destructive';
    default:
      return 'outline';
  }
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

function getDayOfWeek(dateStr: string): number {
  const date = new Date(dateStr);
  return date.getDay();
}

interface HeatmapProps {
  dailyActivity: DailyActivityDTO[];
}

function ActivityHeatmap({ dailyActivity }: HeatmapProps) {
  const { weeks, maxActivity } = useMemo(() => {
    const max = Math.max(...dailyActivity.map(d => d.turn_count + d.message_count), 1);
    const weeksMap: DailyActivityDTO[][] = [];
    let currentWeek: DailyActivityDTO[] = [];

    dailyActivity.forEach((day, index) => {
      const dayOfWeek = getDayOfWeek(day.date);

      if (index === 0) {
        for (let i = 0; i < dayOfWeek; i++) {
          currentWeek.push({ date: '', turn_count: 0, message_count: 0, task_count: 0 });
        }
      }

      if (dayOfWeek === 0 && currentWeek.length > 0) {
        weeksMap.push(currentWeek);
        currentWeek = [];
      }

      currentWeek.push(day);
    });

    if (currentWeek.length > 0) {
      weeksMap.push(currentWeek);
    }

    return { weeks: weeksMap, maxActivity: max };
  }, [dailyActivity]);

  const dayLabels = ['', 'Mon', '', 'Wed', '', 'Fri', ''];

  return (
    <div className="overflow-x-auto">
      <div className="flex gap-[3px]">
        <div className="flex flex-col gap-[3px] pr-1">
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
                {week.map((day, dayIndex) => {
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
            <span className="text-sm truncate">{agent.agent_profile_name}</span>
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


export function StatsPageClient({ stats, error, workspaceId }: StatsPageClientProps) {
  // No workspace selected
  if (!workspaceId) {
    return (
      <div className="h-screen w-full flex flex-col bg-background">
        <header className="flex items-center gap-3 p-4 pb-3">
          <Link href="/" className="text-2xl font-bold hover:opacity-80">KanDev</Link>
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
          <Link href="/" className="text-2xl font-bold hover:opacity-80">KanDev</Link>
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
          <Link href="/" className="text-2xl font-bold hover:opacity-80">KanDev</Link>
          <span className="text-muted-foreground">/</span>
          <span className="text-muted-foreground">Statistics</span>
        </header>
        <div className="flex-1 flex items-center justify-center">
          <p className="text-muted-foreground">No stats available.</p>
        </div>
      </div>
    );
  }

  const { global, task_stats, daily_activity, agent_usage, git_stats } = stats;
  const completionRate = global.total_tasks > 0
    ? Math.round((global.completed_tasks / global.total_tasks) * 100)
    : 0;
  const hasGitStats = git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      {/* Header matching kanban style */}
      <header className="flex items-center gap-3 p-4 pb-3 shrink-0">
        <Link href="/" className="text-2xl font-bold hover:opacity-80">KanDev</Link>
        <span className="text-muted-foreground">/</span>
        <span className="text-muted-foreground">Statistics</span>
      </header>

      {/* Scrollable content area */}
      <div className="flex-1 overflow-auto">
        <div className="max-w-6xl mx-auto p-6 space-y-6">
          {/* Overview Cards */}
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
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
      </div>

      {/* Activity and Agents Row */}
      <div className="grid gap-4 lg:grid-cols-2 mb-8">
        {daily_activity && daily_activity.length > 0 && (
          <Card className="rounded-sm">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Activity (last 90 days)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ActivityHeatmap dailyActivity={daily_activity} />
            </CardContent>
          </Card>
        )}

        <Card className="rounded-sm">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Top Agents</CardTitle>
          </CardHeader>
          <CardContent>
            <AgentUsageList agentUsage={agent_usage} />
          </CardContent>
        </Card>
      </div>

      {/* Task Details Table */}
      <div>
        <h3 className="text-sm font-medium mb-3">Task Details</h3>
        <div className="rounded-sm border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Task</TableHead>
                <TableHead>State</TableHead>
                <TableHead className="text-right">Turns</TableHead>
                <TableHead className="text-right">Messages</TableHead>
                <TableHead className="text-right">Duration</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {task_stats.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                    No tasks yet.
                  </TableCell>
                </TableRow>
              ) : (
                task_stats.map((task) => (
                  <TableRow key={task.task_id}>
                    <TableCell className="font-medium max-w-[300px] truncate" title={task.task_title}>
                      {task.task_title}
                    </TableCell>
                    <TableCell>
                      <Badge variant={getStateBadgeVariant(task.state)}>{task.state}</Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{task.turn_count}</TableCell>
                    <TableCell className="text-right tabular-nums">{task.message_count}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatDuration(task.total_duration_ms)}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>
        </div>
      </div>
    </div>
  );
}
