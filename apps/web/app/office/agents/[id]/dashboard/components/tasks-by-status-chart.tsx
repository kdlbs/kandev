import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import { ChartLegend } from "./run-activity-chart";
import type { AgentTaskStatusDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  return (
    <Card data-testid="tasks-by-status-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{t("office:tasksByStatus")}</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars
          rows={rowsFromDays(days)}
          heightPx={120}
          ariaLabel={t("office:tasksWorkedOnByStatus")}
        />
        <ChartLegend
          items={[
            { label: t("office:statusTodo"), className: "bg-slate-400" },
            { label: t("office:inProgress"), className: "bg-blue-500" },
            { label: t("office:inReview"), className: "bg-violet-500" },
            { label: t("office:statusDone"), className: "bg-emerald-500" },
            { label: t("office:routeBlocked"), className: "bg-orange-500" },
            { label: t("office:statusCancelled"), className: "bg-zinc-400" },
            { label: t("office:statusBacklog"), className: "bg-slate-200" },
          ]}
        />
      </CardContent>
    </Card>
  );
}
