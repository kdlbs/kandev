import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import { ChartLegend } from "./run-activity-chart";
import type { AgentTaskPriorityDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  return (
    <Card data-testid="tasks-by-priority-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{t("office:tasksByPriority")}</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars
          rows={rowsFromDays(days)}
          heightPx={120}
          ariaLabel={t("office:tasksWorkedOnByPriority")}
        />
        <ChartLegend
          items={[
            { label: t("office:priorityCritical"), className: "bg-red-600" },
            { label: t("office:priorityHigh"), className: "bg-orange-500" },
            { label: t("office:priorityMedium"), className: "bg-amber-400" },
            { label: t("office:priorityLow"), className: "bg-blue-400" },
          ]}
        />
      </CardContent>
    </Card>
  );
}
