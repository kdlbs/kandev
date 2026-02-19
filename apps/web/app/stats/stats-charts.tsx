"use client";

import { useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { DailyActivityDTO, AgentUsageDTO, CompletedTaskActivityDTO } from "@/lib/types/http";

function formatMonthLabel(date: Date): string {
  return date.toLocaleDateString("en-US", { month: "short", year: "2-digit" });
}

function formatWeekLabel(date: Date): string {
  return date.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

function getHeatmapColor(intensity: number): string {
  if (intensity === 0) return "bg-muted";
  if (intensity < 0.25) return "bg-emerald-500/30";
  if (intensity < 0.5) return "bg-emerald-500/50";
  if (intensity < 0.75) return "bg-emerald-500/70";
  return "bg-emerald-500/90";
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

function HeatmapGrid({ weeks, maxActivity }: { weeks: DailyActivityDTO[][]; maxActivity: number }) {
  return (
    <TooltipProvider delayDuration={100}>
      <div className="flex gap-[3px]">
        {weeks.map((week, weekIndex) => (
          <div key={weekIndex} className="flex flex-col gap-[3px]">
            {Array.from({ length: 7 }).map((_, dayIndex) => {
              const day = week[dayIndex] ?? {
                date: "",
                turn_count: 0,
                message_count: 0,
                task_count: 0,
              };
              const activity = day.turn_count + day.message_count;
              const intensity = activity / maxActivity;

              if (!day.date) {
                return <div key={dayIndex} className="h-[10px] w-[10px]" />;
              }

              return (
                <Tooltip key={day.date}>
                  <TooltipTrigger asChild>
                    <div
                      className={`h-[10px] w-[10px] rounded-[2px] ${getHeatmapColor(intensity)}`}
                    />
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
  );
}

export function ActivityHeatmap({ dailyActivity }: { dailyActivity: DailyActivityDTO[] }) {
  const { weeks, maxActivity, monthLabels } = useMemo(() => {
    if (!dailyActivity || dailyActivity.length === 0) {
      return {
        weeks: [] as DailyActivityDTO[][],
        maxActivity: 1,
        monthLabels: [] as { index: number; label: string }[],
      };
    }

    const max = Math.max(...dailyActivity.map((d) => d.turn_count + d.message_count), 1);
    const startDate = new Date(`${dailyActivity[0].date}T00:00:00`);
    const startDay = startDate.getDay();
    const padded: DailyActivityDTO[] = [];

    for (let i = 0; i < startDay; i++) {
      padded.push({ date: "", turn_count: 0, message_count: 0, task_count: 0 });
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
        monthMarkers.push({ index, label: date.toLocaleDateString("en-US", { month: "short" }) });
        lastMonth = month;
      }
    });

    return { weeks: weeksMap, maxActivity: max, monthLabels: monthMarkers };
  }, [dailyActivity]);

  const dayLabels = ["", "Mon", "", "Wed", "", "Fri", ""];

  return (
    <div className="overflow-x-auto">
      <div className="min-w-max">
        <div className="ml-8 h-4 flex items-end gap-3 text-[10px] text-muted-foreground">
          {monthLabels.map((label) => (
            <div
              key={`${label.label}-${label.index}`}
              style={{ marginLeft: `${label.index * 14}px` }}
            >
              {label.label}
            </div>
          ))}
        </div>

        <div className="flex gap-[3px]">
          <div className="flex flex-col gap-[3px] pr-2">
            {dayLabels.map((label, i) => (
              <div
                key={i}
                className="h-[10px] w-6 text-[10px] text-muted-foreground flex items-center"
              >
                {label}
              </div>
            ))}
          </div>

          <HeatmapGrid weeks={weeks} maxActivity={maxActivity} />
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

export function AgentUsageList({ agentUsage }: { agentUsage: AgentUsageDTO[] }) {
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

type CompletionBucket = "day" | "week" | "month";

function BucketBarChart({
  series,
  maxCount,
}: {
  series: { label: string; count: number; date: Date }[];
  maxCount: number;
}) {
  return (
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
  );
}

function useBucketedSeries(safeCompleted: CompletedTaskActivityDTO[], bucket: CompletionBucket) {
  return useMemo(() => {
    if (safeCompleted.length === 0) {
      return [] as { label: string; count: number; date: Date }[];
    }

    const toDate = (dateStr: string) => {
      const [year, month, day] = dateStr.split("-").map(Number);
      return new Date(Date.UTC(year, month - 1, day));
    };

    const data = safeCompleted
      .map((item) => ({ date: toDate(item.date), count: item.completed_tasks }))
      .filter((item) => Number.isFinite(item.date.getTime()));

    const buckets = new Map<string, { label: string; count: number; date: Date }>();

    data.forEach((item) => {
      if (bucket === "day") {
        const key = item.date.toISOString().slice(0, 10);
        buckets.set(key, { label: formatDate(key), count: item.count, date: item.date });
        return;
      }

      if (bucket === "week") {
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
      const key = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
      const existing = buckets.get(key);
      const nextCount = (existing?.count ?? 0) + item.count;
      buckets.set(key, { label: formatMonthLabel(d), count: nextCount, date: d });
    });

    return Array.from(buckets.values()).sort((a, b) => a.date.getTime() - b.date.getTime());
  }, [bucket, safeCompleted]);
}

export function CompletedTasksChart({
  completedActivity,
}: {
  completedActivity: CompletedTaskActivityDTO[];
}) {
  const [bucket, setBucket] = useState<CompletionBucket>("day");
  const safeCompleted = useMemo(() => completedActivity ?? [], [completedActivity]);
  const series = useBucketedSeries(safeCompleted, bucket);
  const maxCount = Math.max(...series.map((item) => item.count), 1);

  if (safeCompleted.length === 0) {
    return <div className="text-sm text-muted-foreground py-4">No completed task data yet.</div>;
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span className="uppercase tracking-wider text-[10px]">Bucket</span>
        {(["day", "week", "month"] as CompletionBucket[]).map((b) => (
          <Button
            key={b}
            type="button"
            size="sm"
            variant={bucket === b ? "secondary" : "outline"}
            className="h-7 px-2 font-mono text-[11px] cursor-pointer"
            onClick={() => setBucket(b)}
          >
            {b.charAt(0).toUpperCase() + b.slice(1)}
          </Button>
        ))}
      </div>

      <BucketBarChart series={series} maxCount={maxCount} />

      <div className="flex items-center justify-between text-[10px] text-muted-foreground font-mono">
        <span>{series[0]?.label ?? ""}</span>
        <span>{series[series.length - 1]?.label ?? ""}</span>
      </div>
    </div>
  );
}

export function MostProductiveSummary({
  completedActivity,
}: {
  completedActivity: CompletedTaskActivityDTO[];
}) {
  const safeCompleted = useMemo(() => completedActivity ?? [], [completedActivity]);

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
      const [year, month, day] = item.date.split("-").map(Number);
      const date = new Date(Date.UTC(year, month - 1, day));
      weekdayTotals[date.getUTCDay()] += item.completed_tasks;
      monthTotals[date.getUTCMonth()] += item.completed_tasks;
      yearTotals.set(year, (yearTotals.get(year) ?? 0) + item.completed_tasks);
    });

    const maxWeekday = weekdayTotals.reduce(
      (best, value, idx) => (value > best.value ? { idx, value } : best),
      { idx: 0, value: weekdayTotals[0] ?? 0 },
    );

    const maxMonth = monthTotals.reduce(
      (best, value, idx) => (value > best.value ? { idx, value } : best),
      { idx: 0, value: monthTotals[0] ?? 0 },
    );

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

  const weekdayNames = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const monthNames = [
    "Jan",
    "Feb",
    "Mar",
    "Apr",
    "May",
    "Jun",
    "Jul",
    "Aug",
    "Sep",
    "Oct",
    "Nov",
    "Dec",
  ];

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
          {stats.maxYear.year || "\u2014"} · {stats.maxYear.value}
        </span>
      </div>
    </div>
  );
}
