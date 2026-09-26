/**
 * Run Activity stacked-bars chart. One bar per day in the window;
 * each bar shows every backend outcome bucket.
 */

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import type { AgentRunActivityDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";
import { useTranslation } from "react-i18next";

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
      { key: "skipped", value: d.skipped, className: "bg-amber-500" },
      { key: "unclassified", value: d.unclassified, className: "bg-slate-400" },
      { key: "failed", value: d.failed, className: "bg-red-500" },
      { key: "other", value: d.other, className: "bg-muted-foreground/40" },
    ],
  }));
}

export function RunActivityChart({ days }: Props) {
  const { t } = useTranslation();
  const total = days.reduce((sum, d) => sum + d.total, 0);
  return (
    <Card data-testid="run-activity-card">
      <CardHeader className="pb-2">
        <CardTitle className="flex items-baseline justify-between text-sm">
          <span>{t("office:agentRunActivity")}</span>
          <span className="text-xs font-normal text-muted-foreground">
            {t("office:runCount", { count: total })}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars
          rows={rowsFromDays(days)}
          heightPx={120}
          ariaLabel={t("office:agentRunActivity")}
        />
        <ChartLegend
          data-testid="run-activity-legend"
          items={[
            { label: t("office:succeeded"), className: "bg-emerald-500" },
            { label: t("office:runSkipped"), className: "bg-amber-500" },
            { label: t("office:runUnclassified"), className: "bg-slate-400" },
            { label: t("office:failed"), className: "bg-red-500" },
            { label: t("office:other"), className: "bg-muted-foreground/40" },
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
export function ChartLegend({
  items,
  "data-testid": dataTestId,
}: {
  items: Array<{ label: string; className: string }>;
  "data-testid"?: string;
}) {
  return (
    <div
      className="flex flex-wrap gap-x-3 gap-y-1 mt-2 text-xs text-muted-foreground"
      data-testid={dataTestId}
    >
      {items.map((item) => (
        <span key={item.label} className="flex items-center gap-1">
          <span className={`inline-block w-2 h-2 rounded-sm ${item.className}`} />
          {item.label}
        </span>
      ))}
    </div>
  );
}
