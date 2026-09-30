import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import type { AutonomySpend as AutonomySpendRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { formatSubcentsUsd } from "@/lib/coordinator/autonomy";
import { spendView } from "@/lib/coordinator/autonomy-strip";

type Props = {
  spend: AutonomySpendRead;
  /** The settings section names a missing ceiling with a pill; the strip never does. */
  noCeilingPill?: boolean;
};

/** The 24-hour spend text and pill shared by the strip and the settings section. */
export function AutonomySpend({ spend, noCeilingPill = false }: Props) {
  const { t } = useTranslation();
  const view = spendView(spend);
  if (view.kind === "unmeasurable") {
    return (
      <span data-testid="autonomy-spend" data-spend="unmeasurable">
        {view.degraded ? t("coordinator:spendUnknown") : t("coordinator:spendUnavailable")}
      </span>
    );
  }
  if (view.kind === "no-ceiling") {
    return (
      <span
        className="inline-flex flex-wrap items-center gap-2"
        data-testid="autonomy-spend"
        data-spend="no-ceiling"
      >
        {t("coordinator:spendLineNoCeiling", { amount: formatSubcentsUsd(view.windowSubcents) })}
        {noCeilingPill && (
          <Badge variant="secondary" data-testid="autonomy-spend-pill" data-pill="no-ceiling">
            {t("coordinator:spendNoCeiling")}
          </Badge>
        )}
      </span>
    );
  }
  return (
    <span
      className="inline-flex flex-wrap items-center gap-2"
      data-testid="autonomy-spend"
      data-spend="ceiling"
    >
      {t("coordinator:spendLine", {
        amount: formatSubcentsUsd(view.windowSubcents),
        ceiling: formatSubcentsUsd(view.ceilingSubcents),
      })}
      <Badge
        variant={view.over ? "destructive" : "secondary"}
        data-testid="autonomy-spend-pill"
        data-pill={view.over ? "over" : "inside"}
      >
        {view.over ? t("coordinator:spendOver") : t("coordinator:spendInside")}
      </Badge>
    </span>
  );
}
