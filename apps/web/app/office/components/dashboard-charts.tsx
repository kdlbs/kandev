"use client";

import { Card } from "@kandev/ui/card";
import type { RunActivityDay } from "@/lib/state/slices/office/types";

function formatDateLabel(dateStr: string): string {
  const date = new Date(`${dateStr}T00:00:00`);
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

type Segment = { value: number; color: string };

function StackedBar({ segments, maxTotal }: { segments: Segment[]; maxTotal: number }) {
  const total = segments.reduce((sum, s) => sum + s.value, 0);
  const heightPct = maxTotal > 0 ? Math.max((total / maxTotal) * 100, 2) : 0;
  return (
    <div className="flex-1 flex flex-col-reverse" style={{ height: `${heightPct}%` }}>
      {segments
        .filter((s) => s.value > 0)
        .map((s, i) => (
          <div
            key={i}
            className="w-full min-h-[1px]"
            style={{ flex: s.value, backgroundColor: s.color }}
          />
        ))}
    </div>
  );
}

type ChartXAxisProps = {
  dates: string[];
};

function ChartXAxis({ dates }: ChartXAxisProps) {
  const label = (idx: number) => (dates[idx] ? formatDateLabel(dates[idx]) : "");

  return (
    <div className="relative h-4 mt-1">
      <span className="absolute left-0 text-[10px] text-muted-foreground font-mono">
        {label(0)}
      </span>
      <span className="absolute left-1/2 -translate-x-1/2 text-[10px] text-muted-foreground font-mono">
        {label(6)}
      </span>
      <span className="absolute right-0 text-[10px] text-muted-foreground font-mono">
        {label(13)}
      </span>
    </div>
  );
}

export function RunActivityChart({ data }: { data: RunActivityDay[] }) {
  const maxTotal = Math.max(...data.map((d) => d.succeeded + d.failed + d.other), 1);

  return (
    <Card className="p-4">
      <div className="mb-3">
        <h3 className="text-sm font-semibold">Run Activity</h3>
        <p className="text-xs text-muted-foreground">Last 14 days</p>
      </div>
      <div className="h-28 flex items-end gap-[2px]">
        {data.map((day, i) => (
          <StackedBar
            key={i}
            maxTotal={maxTotal}
            segments={[
              { value: day.succeeded, color: "#10b981" },
              { value: day.failed, color: "#ef4444" },
              { value: day.other, color: "#6b7280" },
            ]}
          />
        ))}
      </div>
      <ChartXAxis dates={data.map((d) => d.date)} />
      <div className="flex items-center gap-3 mt-2">
        <LegendDot color="#10b981" label="Succeeded" />
        <LegendDot color="#ef4444" label="Failed" />
        <LegendDot color="#6b7280" label="Other" />
      </div>
    </Card>
  );
}

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1">
      <div className="h-2 w-2 rounded-full shrink-0" style={{ backgroundColor: color }} />
      <span className="text-[10px] text-muted-foreground">{label}</span>
    </div>
  );
}

function successRateColor(rate: number, hasRuns: boolean): string {
  if (!hasRuns) return "#6b7280";
  if (rate >= 0.8) return "#10b981";
  if (rate >= 0.5) return "#eab308";
  return "#ef4444";
}

export function SuccessRateChart({ data }: { data: RunActivityDay[] }) {
  return (
    <Card className="p-4">
      <div className="mb-3">
        <h3 className="text-sm font-semibold">Success Rate</h3>
        <p className="text-xs text-muted-foreground">Last 14 days</p>
      </div>
      <div className="h-28 flex items-end gap-[2px]">
        {data.map((day, i) => {
          const total = day.succeeded + day.failed + day.other;
          const rate = total > 0 ? day.succeeded / total : 0;
          const heightPct = total > 0 ? Math.max(rate * 100, 2) : 0;
          const color = successRateColor(rate, total > 0);
          return (
            <div
              key={i}
              className="flex-1"
              style={{ height: `${heightPct}%`, backgroundColor: color }}
            />
          );
        })}
      </div>
      <ChartXAxis dates={data.map((d) => d.date)} />
      <div className="flex items-center gap-3 mt-2">
        <LegendDot color="#10b981" label=">= 80%" />
        <LegendDot color="#eab308" label=">= 50%" />
        <LegendDot color="#ef4444" label="< 50%" />
        <LegendDot color="#6b7280" label="No runs" />
      </div>
    </Card>
  );
}
