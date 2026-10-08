"use client";

import { useTranslation } from "react-i18next";
import type { GoalMeasure, GoalMeasures } from "@/lib/api/domains/coordinator-api";
import { formatTimestampDate } from "@/lib/coordinators/goal-form";

function MeasureValue({ measure }: { measure: GoalMeasure }) {
  const { t } = useTranslation();
  if (measure.direction === "none_no_baseline" || measure.baseline === null) {
    return <span>{t("coordinator:goalNoBaseline")}</span>;
  }
  if (measure.direction === "none_small") return <span>{t("coordinator:goalNoDirection")}</span>;
  const change = t("coordinator:goalMeasureChange", {
    baseline: measure.baseline,
    current: measure.current,
  });
  const direction = t(
    measure.direction === "up" ? "coordinator:goalDirectionUp" : "coordinator:goalDirectionDown",
  );
  return (
    <span>
      {change} {direction}
    </span>
  );
}

type GoalMeasuresListProps = {
  measures: GoalMeasures;
  setAt: string;
};

export function GoalMeasuresList({ measures, setAt }: GoalMeasuresListProps) {
  const { t, i18n } = useTranslation();
  const rows: [string, GoalMeasure][] = [
    [t("coordinator:goalMeasureOpenTasks"), measures.open_tasks],
    [t("coordinator:goalMeasureApproved"), measures.approved_7d],
    [t("coordinator:goalMeasureRejected"), measures.rejected_7d],
  ];
  return (
    <div className="space-y-1 text-sm" data-testid="goal-measures">
      <p className="text-muted-foreground">
        {t("coordinator:goalSince", { date: formatTimestampDate(setAt, i18n.language) })}
      </p>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
        {rows.map(([label, measure]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd>
              <MeasureValue measure={measure} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
