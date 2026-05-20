import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import { ChartLegend } from "./run-activity-chart";
import type { AgentTaskPriorityDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";

type Props = { days: AgentTaskPriorityDay[] };

function rowsFromDays(days: AgentTaskPriorityDay[]): StackedBarRow[] {
  return days.map((d) => ({
    id: d.date,
    label: formatBarLabel(d.date),
    segments: [
      { key: "critical", value: d.critical, className: "bg-red-600" },
      { key: "high", value: d.high, className: "bg-orange-500" },
      { key: "medium", value: d.medium, className: "bg-amber-400" },
      { key: "low", value: d.low, className: "bg-blue-400" },
    ],
  }));
}

export function TasksByPriorityChart({ days }: Props) {
  return (
    <Card data-testid="tasks-by-priority-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Tasks by priority</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars
          rows={rowsFromDays(days)}
          heightPx={120}
          ariaLabel="Tasks worked on by priority"
        />
        <ChartLegend
          items={[
            { label: "Critical", className: "bg-red-600" },
            { label: "High", className: "bg-orange-500" },
            { label: "Medium", className: "bg-amber-400" },
            { label: "Low", className: "bg-blue-400" },
          ]}
        />
      </CardContent>
    </Card>
  );
}
