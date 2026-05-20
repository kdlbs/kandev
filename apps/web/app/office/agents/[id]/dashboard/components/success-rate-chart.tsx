import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { StackedBars, type StackedBarRow } from "./stacked-bars";
import type { AgentSuccessRateDay } from "@/lib/api/domains/office-extended-api";
import { formatBarLabel } from "./format-date";

type Props = { days: AgentSuccessRateDay[] };

/**
 * Per-day success rate. We render this as a stacked bar where each
 * bar maxes out at 100 — the green segment is `succeeded/total *
 * 100`, the muted segment is the remainder. Days with zero runs are
 * an empty bar (both segments zero) and the chart still draws the
 * spacer so the timeline lines up with the run-activity chart.
 */
function rowsFromDays(days: AgentSuccessRateDay[]): StackedBarRow[] {
  return days.map((d) => {
    if (d.total === 0) {
      return {
        id: d.date,
        label: formatBarLabel(d.date),
        segments: [
          { key: "succeeded", value: 0, className: "bg-emerald-500" },
          { key: "remainder", value: 0, className: "bg-muted/30" },
        ],
      };
    }
    const successPct = Math.round((d.succeeded / d.total) * 100);
    return {
      id: d.date,
      label: formatBarLabel(d.date),
      segments: [
        { key: "succeeded", value: successPct, className: "bg-emerald-500" },
        { key: "remainder", value: 100 - successPct, className: "bg-muted/30" },
      ],
    };
  });
}

export function SuccessRateChart({ days }: Props) {
  const totals = days.reduce(
    (acc, d) => ({ succeeded: acc.succeeded + d.succeeded, total: acc.total + d.total }),
    { succeeded: 0, total: 0 },
  );
  const overall = totals.total === 0 ? 0 : Math.round((totals.succeeded / totals.total) * 100);
  return (
    <Card data-testid="success-rate-card">
      <CardHeader className="pb-2">
        <CardTitle className="flex items-baseline justify-between text-sm">
          <span>Success rate</span>
          <span className="text-xs font-normal text-muted-foreground">
            {totals.total === 0 ? "—" : `${overall}% over ${totals.total} runs`}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <StackedBars
          rows={rowsFromDays(days)}
          heightPx={120}
          maxValue={100}
          ariaLabel="Daily success rate"
        />
      </CardContent>
    </Card>
  );
}
