/**
 * Run Activity stacked-bars chart. One bar per day in the window;
 * each bar stacks succeeded (green) / failed (red) / other (muted).
 */

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import type { AgentRunActivityDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";

type Props = { days: AgentRunActivityDay[] };

/**
 * Builds the stacked-bar rows for the run-activity chart from the
 * SSR-supplied per-day counts. The result is a stable, ordered list
 * the chart primitive renders verbatim.
 */
function rowsFromDays(days: AgentRunActivityDay[]): StackedBarRow[] {
  return days.map((d) => ({
    id: d.date,
    label: formatBarLabel(d.date),
    segments: [
      { key: "succeeded", value: d.succeeded, className: "bg-emerald-500" },
      { key: "failed", value: d.failed, className: "bg-red-500" },
      { key: "other", value: d.other, className: "bg-muted-foreground/40" },
    ],
  }));
}

export function RunActivityChart({ days }: Props) {
  const total = days.reduce((sum, d) => sum + d.total, 0);
  return (
    <Card data-testid="run-activity-card">
      <CardHeader className="pb-2">
        <CardTitle className="flex items-baseline justify-between text-sm">
          <span>Run activity</span>
          <span className="text-xs font-normal text-muted-foreground">
            {total} run{total === 1 ? "" : "s"}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars rows={rowsFromDays(days)} heightPx={120} ariaLabel="Run activity" />
        <ChartLegend
          items={[
            { label: "Succeeded", className: "bg-emerald-500" },
            { label: "Failed", className: "bg-red-500" },
            { label: "Other", className: "bg-muted-foreground/40" },
          ]}
        />
      </CardContent>
    </Card>
  );
}

/**
 * Tiny legend rendered under each chart. Inline so the chart cards
 * don't accumulate one-off helper components.
 */
export function ChartLegend({ items }: { items: Array<{ label: string; className: string }> }) {
  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1 mt-2 text-xs text-muted-foreground">
      {items.map((item) => (
        <span key={item.label} className="flex items-center gap-1">
          <span className={`inline-block w-2 h-2 rounded-sm ${item.className}`} />
          {item.label}
        </span>
      ))}
    </div>
  );
}
