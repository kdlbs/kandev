import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import { ChartLegend } from "./run-activity-chart";
import type { AgentTaskStatusDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";

type Props = { days: AgentTaskStatusDay[] };

function rowsFromDays(days: AgentTaskStatusDay[]): StackedBarRow[] {
  return days.map((d) => ({
    id: d.date,
    label: formatBarLabel(d.date),
    segments: [
      { key: "todo", value: d.todo, className: "bg-slate-400" },
      { key: "in_progress", value: d.in_progress, className: "bg-blue-500" },
      { key: "in_review", value: d.in_review, className: "bg-violet-500" },
      { key: "done", value: d.done, className: "bg-emerald-500" },
      { key: "blocked", value: d.blocked, className: "bg-orange-500" },
      { key: "cancelled", value: d.cancelled, className: "bg-zinc-400" },
      { key: "backlog", value: d.backlog, className: "bg-slate-200" },
    ],
  }));
}

export function TasksByStatusChart({ days }: Props) {
  return (
    <Card data-testid="tasks-by-status-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Tasks by status</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars
          rows={rowsFromDays(days)}
          heightPx={120}
          ariaLabel="Tasks worked on by status"
        />
        <ChartLegend
          items={[
            { label: "Todo", className: "bg-slate-400" },
            { label: "In progress", className: "bg-blue-500" },
            { label: "In review", className: "bg-violet-500" },
            { label: "Done", className: "bg-emerald-500" },
            { label: "Blocked", className: "bg-orange-500" },
            { label: "Cancelled", className: "bg-zinc-400" },
            { label: "Backlog", className: "bg-slate-200" },
          ]}
        />
      </CardContent>
    </Card>
  );
}
